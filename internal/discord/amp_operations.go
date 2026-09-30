package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/disgoorg/snowflake/v2"
)

func (c *Client) executeAMPControlOperation(
	applicationID snowflake.ID,
	interactionToken string,
	instance amp.ManagedInstance,
	commandOperation ampCommandOperation,
	bypassPlayerProtection bool,
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
				i18n.Choose("❌ Não foi possível reservar **%s** para a operação.\n", "❌ Could not reserve **%s** for the operation.\n")+
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
				i18n.Choose("⏳ **%s** já possui uma operação em andamento.\n", "⏳ **%s** already has an operation in progress.\n")+
					i18n.Choose("Operação atual: `%s`\n", "Current operation: `%s`\n")+
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

	protection := c.checkAMPPlayerProtection(
		ctx,
		instance,
		commandOperation,
		bypassPlayerProtection,
	)
	if !protection.Allowed {
		c.log.Warn().
			Err(protection.Err).
			Str("instance", instance.Name).
			Str("operation", string(commandOperation)).
			Int("players", protection.PlayerCount).
			Msg("Operação AMP recusada pela proteção de jogadores")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			protection.RefusalMessage(instance, commandOperation),
		)

		return
	}

	if protection.Bypassed &&
		(protection.PlayerCount > 0 || protection.Err != nil) {
		c.log.Warn().
			Err(protection.Err).
			Str("instance", instance.Name).
			Str("operation", string(commandOperation)).
			Int("players", protection.PlayerCount).
			Msg("Proteção de jogadores ignorada por usuário privilegiado")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			protection.BypassMessage(instance, commandOperation),
		)
	}

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
				i18n.Choose("a instância foi iniciada, mas o jogo não pôde ser iniciado: %w", "the instance was started, but the game could not be started: %w"),
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
			i18n.Choose("🔵 O jogo de **%s** já está em transição.\n", "🔵 The game for **%s** is already in transition.\n")+
				"Estado atual: `%s`",
			displayName,
			status.State.String(),
		), nil

	case amp.ApplicationPhaseSuspended:
		return fmt.Sprintf(
			i18n.Choose("🟠 O jogo de **%s** está suspenso e não foi iniciado.", "🟠 The game for **%s** is suspended and was not started."),
			displayName,
		), nil
	}

	if err := c.ampClient.StartApplicationUntilReady(
		ctx,
		instance.APIURL,
		ampApplicationRetryPeriod,
	); err != nil {
		return "", fmt.Errorf(
			i18n.Choose("não foi possível iniciar o processo do jogo: %w", "could not start the game process: %w"),
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
			i18n.Choose("a aplicação está no estado de falha %s", "the application is in failed state %s"),
			status.State.String(),
		)

	case amp.ApplicationPhaseSuspended:
		return "", errors.New(i18n.Choose("a aplicação está suspensa", "the application is suspended"))

	case amp.ApplicationPhaseUnknown:
		return "", fmt.Errorf(
			i18n.Choose("o estado da aplicação não é reconhecido: %s", "the application state is not recognized: %s"),
			status.State.String(),
		)
	}

	if err := c.ampClient.StopApplication(
		ctx,
		instance.APIURL,
	); err != nil {
		return "", fmt.Errorf(
			i18n.Choose("não foi possível parar o processo do jogo: %w", "could not stop the game process: %w"),
			err,
		)
	}

	return fmt.Sprintf(
		i18n.Choose("💤 O processo do jogo de **%s** foi parado.\n", "💤 The game process for **%s** was stopped.\n")+
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
			i18n.Choose("⚫ A instância AMP de **%s** está Offline.\n", "⚫ The AMP instance for **%s** is Offline.\n")+
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
			i18n.Choose("a aplicação está no estado de falha %s", "the application is in failed state %s"),
			status.State.String(),
		)

	case amp.ApplicationPhaseSuspended:
		return "", errors.New(i18n.Choose("a aplicação está suspensa", "the application is suspended"))

	case amp.ApplicationPhaseUnknown:
		return "", fmt.Errorf(
			i18n.Choose("o estado da aplicação não é reconhecido: %s", "the application state is not recognized: %s"),
			status.State.String(),
		)
	}

	if err := c.ampClient.RestartApplication(
		ctx,
		instance.APIURL,
	); err != nil {
		return "", fmt.Errorf(
			i18n.Choose("não foi possível reiniciar o processo do jogo: %w", "could not restart the game process: %w"),
			err,
		)
	}

	return fmt.Sprintf(
		i18n.Choose("✅ O processo do jogo de **%s** foi reiniciado.", "✅ The game process for **%s** was restarted."),
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
			i18n.Choose("não foi possível atualizar a instalação AMP: %w", "could not update the AMP installation: %w"),
			err,
		)
	}

	return fmt.Sprintf(
		i18n.Choose("✅ A instalação AMP de **%s** foi atualizada.", "✅ The AMP installation for **%s** was updated."),
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
			i18n.Choose("não foi possível consultar o estado do jogo: %w", "could not query the game state: %w"),
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
			i18n.Choose("não foi possível atualizar a lista de instâncias: %w", "could not refresh the instance list: %w"),
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
		i18n.Choose("a instância %q não foi encontrada", "instance %q was not found"),
		instanceName,
	)
}

func ampOperationProgressMessage(
	operation ampCommandOperation,
) string {
	switch operation {
	case ampCommandOperationStart:
		return i18n.Choose("⏳ Iniciando", "⏳ Starting")

	case ampCommandOperationStop:
		return i18n.Choose("⏳ Colocando em modo Idle", "⏳ Placing in Idle mode")

	case ampCommandOperationRestart:
		return i18n.Choose("🔄 Reiniciando o processo do jogo de", "🔄 Restarting the game process for")

	case ampCommandOperationShutdown:
		return i18n.Choose("⚫ Desligando completamente a instância AMP de", "⚫ Shutting down the AMP instance for")

	case ampCommandOperationUpdate:
		return i18n.Choose("⬆️ Atualizando a instalação AMP de", "⬆️ Updating the AMP instance for")

	default:
		return i18n.Choose("⏳ Executando uma operação em", "⏳ Running an operation on")
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
