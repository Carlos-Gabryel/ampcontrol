package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const (
	ampDiscoveryTimeout       = 15 * time.Second
	ampControlTimeout         = 14 * time.Minute
	ampApplicationTimeout     = 20 * time.Second
	ampApplicationRetryPeriod = 3 * time.Second
)

type ampCommandOperation string

const (
	ampCommandOperationStart    ampCommandOperation = "start"
	ampCommandOperationStop     ampCommandOperation = "stop"
	ampCommandOperationRestart  ampCommandOperation = "restart"
	ampCommandOperationShutdown ampCommandOperation = "shutdown"
	ampCommandOperationUpdate   ampCommandOperation = "update"
)

type ampInstanceStatusView struct {
	Instance          amp.ManagedInstance
	ApplicationStatus *amp.ApplicationStatus
	ApplicationError  error
	PlayerCounts      *amp.PlayerCounts
	PlayerError       error
}

func (c *Client) handleAMPCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if data.SubCommandName == nil {
		c.sendInteractionMessage(
			event,
			"⚠️ Nenhum subcomando foi informado.",
		)

		return
	}

	switch *data.SubCommandName {
	case "status":
		c.handleAMPStatusCommand(event)

	case "iniciar":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationStart,
		)

	case "parar":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationStop,
		)

	case "reiniciar":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationRestart,
		)

	case "desligar":
		if !ampCommandConfirmed(data) {
			c.sendInteractionMessage(
				event,
				"⚠️ O desligamento completo da instância não foi confirmado.",
			)

			return
		}

		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationShutdown,
		)

	case "atualizar":
		if !ampCommandConfirmed(data) {
			c.sendInteractionMessage(
				event,
				"⚠️ A atualização da instalação AMP não foi confirmada.",
			)

			return
		}

		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationUpdate,
		)

	default:
		c.sendInteractionMessage(
			event,
			"⚠️ Subcomando AMP não reconhecido.",
		)
	}
}

func ampCommandAuthorized(
	member *disgoDiscord.ResolvedMember,
) bool {
	return member != nil &&
		member.Permissions.Has(
			disgoDiscord.PermissionAdministrator,
		)
}

func ampCommandConfirmed(
	data disgoDiscord.SlashCommandInteractionData,
) bool {
	confirmation, exists := data.OptString(
		"confirmar",
	)

	return exists && confirmation == "sim"
}

func (c *Client) handleAMPStatusCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	c.requestStatusRefresh()
	c.updateInteractionMessage(
		event,
		"✅ O painel fixo está sendo atualizado.",
	)
	c.deleteInteractionResponseLater(event, 5*time.Second)
}

func (c *Client) collectAMPInstanceStatuses(
	instances []amp.ManagedInstance,
) []ampInstanceStatusView {
	statuses := make(
		[]ampInstanceStatusView,
		len(instances),
	)

	var waitGroup sync.WaitGroup

	for index, instance := range instances {
		statuses[index].Instance = instance

		if !instance.Running {
			continue
		}

		waitGroup.Add(1)

		go func(
			statusIndex int,
			currentInstance amp.ManagedInstance,
		) {
			defer waitGroup.Done()

			if strings.TrimSpace(currentInstance.Game) == "" ||
				strings.EqualFold(currentInstance.Game, "GenericModule") {
				moduleCtx, moduleCancel := context.WithTimeout(
					context.Background(),
					ampApplicationTimeout,
				)
				moduleInfo, moduleErr := c.ampClient.GetModuleInfo(
					moduleCtx,
					currentInstance.APIURL,
				)
				moduleCancel()

				if moduleErr == nil &&
					strings.TrimSpace(moduleInfo.Application) != "" {
					statuses[statusIndex].Instance.Game =
						moduleInfo.Application
				}
			}

			ctx, cancel := context.WithTimeout(
				context.Background(),
				ampApplicationTimeout,
			)
			defer cancel()

			status, err := c.ampClient.GetApplicationStatus(
				ctx,
				currentInstance.APIURL,
			)
			if err != nil {
				statuses[statusIndex].ApplicationError = err

				c.log.Warn().
					Err(err).
					Str("instance", currentInstance.Name).
					Msg("Não foi possível consultar o estado da aplicação")

				return
			}

			statusCopy := status

			statuses[statusIndex].ApplicationStatus =
				&statusCopy

			counts, countsErr := status.PlayerCounts()
			if countsErr != nil {
				statuses[statusIndex].PlayerError = countsErr
				return
			}

			if counts.Current == 0 &&
				status.Phase() == amp.ApplicationPhaseOnline &&
				c.playerCountResolver != nil {
				resolverCtx, resolverCancel := context.WithTimeout(
					context.Background(),
					ampApplicationTimeout,
				)
				resolvedCount, applies, resolverErr :=
					c.playerCountResolver.ResolvePlayerCount(
						resolverCtx,
						currentInstance.Name,
					)
				resolverCancel()

				if applies {
					if resolverErr != nil {
						statuses[statusIndex].PlayerError = resolverErr

						c.log.Warn().
							Err(resolverErr).
							Str("instance", currentInstance.Name).
							Msg("Fallback de jogadores do painel falhou")

						return
					}

					counts.Current = resolvedCount
				}
			}

			countsCopy := counts
			statuses[statusIndex].PlayerCounts = &countsCopy
		}(
			index,
			instance,
		)
	}

	waitGroup.Wait()

	return statuses
}

func (c *Client) handleAMPControlCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	operation ampCommandOperation,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := data.OptString(
		"servidor",
	)
	if !exists || strings.TrimSpace(instanceName) == "" {
		c.updateInteractionMessage(
			event,
			"⚠️ A instância AMP não foi informada.",
		)

		return
	}

	instance, err := c.resolveAMPInstance(
		instanceName,
	)
	if err != nil {
		c.log.Warn().
			Err(err).
			Str("instance", instanceName).
			Msg("Instância AMP selecionada não foi encontrada")

		c.updateInteractionMessage(
			event,
			"⚠️ A instância selecionada não existe ou não pode ser controlada.",
		)

		return
	}

	if c.instanceHidden(instance.Name) {
		c.updateInteractionMessage(
			event,
			"⛔ Essa instância está oculta dos comandos do Discord.",
		)
		return
	}

	c.updateInteractionMessage(
		event,
		fmt.Sprintf(
			"%s **%s**\nInstância: `%s`",
			ampOperationProgressMessage(operation),
			ampInstanceDisplayName(instance),
			instance.Name,
		),
	)

	go c.executeAMPControlOperation(
		event.ApplicationID(),
		event.Token(),
		instance,
		operation,
	)
}

func (c *Client) executeAMPControlOperation(
	applicationID snowflake.ID,
	interactionToken string,
	instance amp.ManagedInstance,
	commandOperation ampCommandOperation,
) {
	defer c.requestStatusRefresh()

	lease, activeOperation, err :=
		c.acquireAMPCommandOperation(
			instance.Name,
			commandOperation,
		)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instance.Name).
			Str(
				"operation",
				string(commandOperation),
			).
			Msg("Não foi possível adquirir o bloqueio da operação AMP")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			fmt.Sprintf(
				"❌ Não foi possível reservar **%s** para a operação.\n"+
					"Erro: `%s`",
				ampInstanceDisplayName(instance),
				sanitizeAMPError(err),
			),
		)

		return
	}

	if lease == nil {
		activeDescription := "outra operação"

		if activeOperation != nil &&
			strings.TrimSpace(activeOperation.Operation) != "" {
			activeDescription = activeOperation.Operation
		}

		c.log.Warn().
			Str("instance", instance.Name).
			Str(
				"requested_operation",
				string(commandOperation),
			).
			Str(
				"active_operation",
				activeDescription,
			).
			Msg("Operação AMP recusada porque a instância está ocupada")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			fmt.Sprintf(
				"⏳ **%s** já possui uma operação em andamento.\n"+
					"Operação atual: `%s`\n"+
					"Aguarde a conclusão antes de enviar outro comando para esta instância.",
				ampInstanceDisplayName(instance),
				activeDescription,
			),
		)

		return
	}

	c.log.Debug().
		Str("instance", instance.Name).
		Str(
			"operation",
			string(commandOperation),
		).
		Msg("Bloqueio exclusivo da instância adquirido")

	defer func() {
		lease.Release()

		c.log.Debug().
			Str("instance", instance.Name).
			Str(
				"operation",
				string(commandOperation),
			).
			Msg("Bloqueio exclusivo da instância liberado")
	}()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampControlTimeout,
	)
	defer cancel()

	c.log.Info().
		Str("instance", instance.Name).
		Str(
			"operation",
			string(commandOperation),
		).
		Msg("Executando operação AMP")

	resultMessage, err := c.performAMPControlOperation(
		ctx,
		instance,
		commandOperation,
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instance.Name).
			Str(
				"operation",
				string(commandOperation),
			).
			Msg("Operação AMP falhou")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			fmt.Sprintf(
				"❌ Não foi possível concluir a operação em **%s**.\n"+
					"Erro: `%s`",
				ampInstanceDisplayName(instance),
				sanitizeAMPError(err),
			),
		)

		return
	}

	c.log.Info().
		Str("instance", instance.Name).
		Str(
			"operation",
			string(commandOperation),
		).
		Msg("Operação AMP concluída")

	c.updateInteractionMessageByToken(
		applicationID,
		interactionToken,
		fmt.Sprintf(
			"%s\nInstância: `%s`",
			resultMessage,
			instance.Name,
		),
	)
}

func (c *Client) performAMPControlOperation(
	ctx context.Context,
	instance amp.ManagedInstance,
	operation ampCommandOperation,
) (string, error) {
	switch operation {
	case ampCommandOperationStart:
		return c.startAMPApplication(
			ctx,
			instance,
		)

	case ampCommandOperationStop:
		return c.stopAMPApplication(
			ctx,
			instance,
		)

	case ampCommandOperationRestart:
		return c.restartAMPApplication(
			ctx,
			instance,
		)

	case ampCommandOperationShutdown:
		return c.shutdownAMPInstance(
			ctx,
			instance,
		)

	case ampCommandOperationUpdate:
		return c.updateAMPInstance(
			ctx,
			instance,
		)

	default:
		return "", fmt.Errorf(
			"operação AMP desconhecida: %q",
			operation,
		)
	}
}

func (c *Client) startAMPApplication(
	ctx context.Context,
	instance amp.ManagedInstance,
) (string, error) {
	displayName := ampInstanceDisplayName(
		instance,
	)

	if !instance.Running {
		if err := amp.StartInstance(
			ctx,
			instance.Name,
		); err != nil {
			return "", fmt.Errorf(
				"não foi possível iniciar a instância AMP: %w",
				err,
			)
		}

		if err := c.ampClient.StartApplicationUntilReady(
			ctx,
			instance.APIURL,
			ampApplicationRetryPeriod,
		); err != nil {
			return "", fmt.Errorf(
				"a instância foi iniciada, mas o jogo não pôde ser iniciado: %w",
				err,
			)
		}

		return fmt.Sprintf(
			"✅ A instância AMP e o jogo de **%s** foram iniciados.",
			displayName,
		), nil
	}

	status, err := c.getAMPApplicationStatus(
		ctx,
		instance,
	)
	if err != nil {
		return "", err
	}

	switch status.Phase() {
	case amp.ApplicationPhaseOnline:
		return fmt.Sprintf(
			"🟢 O jogo de **%s** já está Online.",
			displayName,
		), nil

	case amp.ApplicationPhaseBusy:
		return fmt.Sprintf(
			"🔵 O jogo de **%s** já está em transição.\n"+
				"Estado atual: `%s`",
			displayName,
			status.State.String(),
		), nil

	case amp.ApplicationPhaseSuspended:
		return fmt.Sprintf(
			"🟠 O jogo de **%s** está suspenso e não foi iniciado.",
			displayName,
		), nil
	}

	if err := c.ampClient.StartApplicationUntilReady(
		ctx,
		instance.APIURL,
		ampApplicationRetryPeriod,
	); err != nil {
		return "", fmt.Errorf(
			"não foi possível iniciar o processo do jogo: %w",
			err,
		)
	}

	return fmt.Sprintf(
		"✅ O jogo de **%s** foi iniciado e saiu do modo Idle.",
		displayName,
	), nil
}

func (c *Client) stopAMPApplication(
	ctx context.Context,
	instance amp.ManagedInstance,
) (string, error) {
	displayName := ampInstanceDisplayName(
		instance,
	)

	if !instance.Running {
		return fmt.Sprintf(
			"⚫ A instância AMP de **%s** já está Offline.",
			displayName,
		), nil
	}

	status, err := c.getAMPApplicationStatus(
		ctx,
		instance,
	)
	if err != nil {
		return "", err
	}

	switch status.Phase() {
	case amp.ApplicationPhaseIdle:
		return fmt.Sprintf(
			"💤 O jogo de **%s** já está em modo Idle.",
			displayName,
		), nil

	case amp.ApplicationPhaseBusy:
		return fmt.Sprintf(
			"🔵 O jogo de **%s** está em transição e não foi parado.\n"+
				"Estado atual: `%s`",
			displayName,
			status.State.String(),
		), nil

	case amp.ApplicationPhaseFailed:
		return "", fmt.Errorf(
			"a aplicação está no estado de falha %s",
			status.State.String(),
		)

	case amp.ApplicationPhaseSuspended:
		return "", fmt.Errorf(
			"a aplicação está suspensa",
		)

	case amp.ApplicationPhaseUnknown:
		return "", fmt.Errorf(
			"o estado da aplicação não é reconhecido: %s",
			status.State.String(),
		)
	}

	if err := c.ampClient.StopApplication(
		ctx,
		instance.APIURL,
	); err != nil {
		return "", fmt.Errorf(
			"não foi possível parar o processo do jogo: %w",
			err,
		)
	}

	return fmt.Sprintf(
		"💤 O processo do jogo de **%s** foi parado.\n"+
			"A instância AMP permanece ligada em modo Idle.",
		displayName,
	), nil
}

func (c *Client) restartAMPApplication(
	ctx context.Context,
	instance amp.ManagedInstance,
) (string, error) {
	displayName := ampInstanceDisplayName(
		instance,
	)

	if !instance.Running {
		return fmt.Sprintf(
			"⚫ A instância AMP de **%s** está Offline.\n"+
				"Use `/amp iniciar` para ligar o servidor.",
			displayName,
		), nil
	}

	status, err := c.getAMPApplicationStatus(
		ctx,
		instance,
	)
	if err != nil {
		return "", err
	}

	switch status.Phase() {
	case amp.ApplicationPhaseIdle:
		return fmt.Sprintf(
			"💤 O jogo de **%s** está em modo Idle.\n"+
				"Use `/amp iniciar` para iniciá-lo.",
			displayName,
		), nil

	case amp.ApplicationPhaseBusy:
		return fmt.Sprintf(
			"🔵 O jogo de **%s** está em transição e não foi reiniciado.\n"+
				"Estado atual: `%s`",
			displayName,
			status.State.String(),
		), nil

	case amp.ApplicationPhaseFailed:
		return "", fmt.Errorf(
			"a aplicação está no estado de falha %s",
			status.State.String(),
		)

	case amp.ApplicationPhaseSuspended:
		return "", fmt.Errorf(
			"a aplicação está suspensa",
		)

	case amp.ApplicationPhaseUnknown:
		return "", fmt.Errorf(
			"o estado da aplicação não é reconhecido: %s",
			status.State.String(),
		)
	}

	if err := c.ampClient.RestartApplication(
		ctx,
		instance.APIURL,
	); err != nil {
		return "", fmt.Errorf(
			"não foi possível reiniciar o processo do jogo: %w",
			err,
		)
	}

	return fmt.Sprintf(
		"✅ O processo do jogo de **%s** foi reiniciado.",
		displayName,
	), nil
}

func (c *Client) shutdownAMPInstance(
	ctx context.Context,
	instance amp.ManagedInstance,
) (string, error) {
	displayName := ampInstanceDisplayName(
		instance,
	)

	if !instance.Running {
		return fmt.Sprintf(
			"⚫ A instância AMP de **%s** já está Offline.",
			displayName,
		), nil
	}

	if err := amp.StopInstance(
		ctx,
		instance.Name,
	); err != nil {
		return "", fmt.Errorf(
			"não foi possível desligar a instância AMP: %w",
			err,
		)
	}

	return fmt.Sprintf(
		"⚫ A instância AMP de **%s** foi desligada completamente.",
		displayName,
	), nil
}

func (c *Client) updateAMPInstance(
	ctx context.Context,
	instance amp.ManagedInstance,
) (string, error) {
	displayName := ampInstanceDisplayName(
		instance,
	)

	if err := amp.UpdateInstance(
		ctx,
		instance.Name,
	); err != nil {
		return "", fmt.Errorf(
			"não foi possível atualizar a instalação AMP: %w",
			err,
		)
	}

	return fmt.Sprintf(
		"✅ A instalação AMP de **%s** foi atualizada.",
		displayName,
	), nil
}

func (c *Client) getAMPApplicationStatus(
	ctx context.Context,
	instance amp.ManagedInstance,
) (amp.ApplicationStatus, error) {
	statusCtx, statusCancel := context.WithTimeout(
		ctx,
		ampApplicationTimeout,
	)
	defer statusCancel()

	status, err := c.ampClient.GetApplicationStatus(
		statusCtx,
		instance.APIURL,
	)
	if err != nil {
		return amp.ApplicationStatus{}, fmt.Errorf(
			"não foi possível consultar o estado do jogo: %w",
			err,
		)
	}

	return status, nil
}

func (c *Client) resolveAMPInstance(
	instanceName string,
) (amp.ManagedInstance, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampDiscoveryTimeout,
	)
	defer cancel()

	instances, err := c.discoverAMPInstances(ctx)
	if err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"não foi possível atualizar a lista de instâncias: %w",
			err,
		)
	}

	instanceName = strings.TrimSpace(
		instanceName,
	)

	for _, instance := range instances {
		if instance.Name == instanceName {
			return instance, nil
		}
	}

	return amp.ManagedInstance{}, fmt.Errorf(
		"a instância %q não foi encontrada",
		instanceName,
	)
}

func (c *Client) deferAMPInteraction(
	event *events.ApplicationCommandInteractionCreate,
) bool {
	err := event.DeferCreateMessage(true)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro adiando resposta do comando AMP")

		return false
	}

	return true
}

func buildAMPStatusEmbeds(
	statuses []ampInstanceStatusView,
	updatedAt time.Time,
) []disgoDiscord.Embed {
	const serversPerRow = 2

	if len(statuses) == 0 {
		return []disgoDiscord.Embed{
			disgoDiscord.NewEmbed().
				WithDescription(
					"Nenhuma instância controlável foi encontrada.",
				).
				WithColor(0x5865F2),
		}
	}

	embeds := make(
		[]disgoDiscord.Embed,
		0,
		(len(statuses)+serversPerRow-1)/serversPerRow+1,
	)

	for rowStart := 0; rowStart < len(statuses); rowStart += serversPerRow {
		row := disgoDiscord.NewEmbed().WithColor(0x5865F2)
		if rowStart == 0 {
			row = row.WithDescription(
				fmt.Sprintf("Atualizado <t:%d:R>", updatedAt.Unix()),
			)
		}

		rowEnd := min(rowStart+serversPerRow, len(statuses))
		for _, statusView := range statuses[rowStart:rowEnd] {
			icon, _ := describeAMPInstanceStatus(statusView)

			game := strings.TrimSpace(statusView.Instance.Game)
			if game == "" {
				game = strings.TrimSpace(statusView.Instance.Module)
			}
			if game == "" {
				game = "Desconhecido"
			}

			uptime := "0 min"
			if statusView.ApplicationStatus != nil {
				uptime = formatAMPUptime(
					statusView.ApplicationStatus.Uptime,
				)
			}

			players := "?/?"
			if statusView.PlayerCounts != nil {
				players = fmt.Sprintf(
					"%d/%d",
					statusView.PlayerCounts.Current,
					statusView.PlayerCounts.Maximum,
				)
			} else if !statusView.Instance.Running {
				players = "0/?"
			}

			row = row.AddField(
				fmt.Sprintf(
					"%s %s",
					icon,
					ampInstanceDisplayName(statusView.Instance),
				),
				fmt.Sprintf(
					"**Jogo:** `%s`\n**Tempo online:** `%s`\n**Jogadores:** `%s`\n──────────────",
					game,
					uptime,
					players,
				),
				true,
			)
		}

		if rowEnd-rowStart == 1 {
			row = row.AddField("\u200b", "\u200b", true)
		}

		embeds = append(embeds, row)
	}

	embeds = append(
		embeds,
		disgoDiscord.NewEmbed().
			WithDescription(
				"**Legenda:**  🟢 Online   •   🟡 Idle   •   🔴 Offline",
			).
			WithColor(0x5865F2),
	)

	return embeds
}

func describeAMPInstanceStatus(
	statusView ampInstanceStatusView,
) (string, string) {
	if !statusView.Instance.Running {
		return "🔴", "Offline"
	}

	if statusView.ApplicationError != nil {
		return "🔴", "Offline — estado indisponível"
	}

	if statusView.ApplicationStatus == nil {
		return "🔴", "Offline — estado indisponível"
	}

	status := *statusView.ApplicationStatus

	switch status.Phase() {
	case amp.ApplicationPhaseIdle:
		return "🟡", "Idle"

	case amp.ApplicationPhaseOnline:
		return "🟢", "Online"

	case amp.ApplicationPhaseBusy:
		return "🟢", fmt.Sprintf(
			"Online — %s",
			status.State.String(),
		)

	case amp.ApplicationPhaseFailed:
		return "🔴", "Offline — falha"

	case amp.ApplicationPhaseSuspended:
		return "🔴", "Offline — suspenso"

	default:
		return "🔴", fmt.Sprintf(
			"Offline — %s",
			status.State.String(),
		)
	}
}

func ampInstanceDisplayName(
	instance amp.ManagedInstance,
) string {
	displayName := strings.TrimSpace(
		instance.FriendlyName,
	)

	if displayName == "" {
		return instance.Name
	}

	return displayName
}

func ampOperationProgressMessage(
	operation ampCommandOperation,
) string {
	switch operation {
	case ampCommandOperationStart:
		return "⏳ Iniciando"

	case ampCommandOperationStop:
		return "⏳ Colocando em modo Idle"

	case ampCommandOperationRestart:
		return "🔄 Reiniciando o processo do jogo de"

	case ampCommandOperationShutdown:
		return "⚫ Desligando completamente a instância AMP de"

	case ampCommandOperationUpdate:
		return "⬆️ Atualizando a instalação AMP de"

	default:
		return "⏳ Executando uma operação em"
	}
}

func sanitizeAMPError(
	err error,
) string {
	message := strings.TrimSpace(
		err.Error(),
	)

	message = strings.ReplaceAll(
		message,
		"`",
		"'",
	)

	if len(message) > 900 {
		message = message[:900]
	}

	return message
}
