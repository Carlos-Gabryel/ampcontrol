package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alabamaamp/palcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

const (
	ampDiscoveryTimeout = 15 * time.Second
	ampControlTimeout   = 14 * time.Minute
)

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
			amp.InstanceOperationStart,
		)

	case "parar":
		c.handleAMPControlCommand(
			event,
			data,
			amp.InstanceOperationStop,
		)

	case "reiniciar":
		c.handleAMPControlCommand(
			event,
			data,
			amp.InstanceOperationRestart,
		)

	case "atualizar":
		confirmation, exists := data.OptString(
			"confirmar",
		)
		if !exists || confirmation != "sim" {
			c.sendInteractionMessage(
				event,
				"⚠️ A atualização não foi confirmada.",
			)

			return
		}

		c.handleAMPControlCommand(
			event,
			data,
			amp.InstanceOperationUpdate,
		)

	default:
		c.sendInteractionMessage(
			event,
			"⚠️ Subcomando AMP não reconhecido.",
		)
	}
}

func (c *Client) handleAMPStatusCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampDiscoveryTimeout,
	)
	defer cancel()

	instances, err := amp.DiscoverInstances(ctx)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro descobrindo instâncias AMP")

		c.updateInteractionMessage(
			event,
			"❌ Não foi possível consultar as instâncias do AMP.",
		)

		return
	}

	c.updateInteractionMessage(
		event,
		buildAMPStatusMessage(instances),
	)
}

func (c *Client) handleAMPControlCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	operation amp.InstanceOperation,
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

	switch operation {
	case amp.InstanceOperationStart:
		if instance.Running {
			c.updateInteractionMessage(
				event,
				fmt.Sprintf(
					"🟢 A instância AMP de **%s** já está ligada.",
					ampInstanceDisplayName(instance),
				),
			)

			return
		}

	case amp.InstanceOperationStop:
		if !instance.Running {
			c.updateInteractionMessage(
				event,
				fmt.Sprintf(
					"⚫ A instância AMP de **%s** já está desligada.",
					ampInstanceDisplayName(instance),
				),
			)

			return
		}
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
	operation amp.InstanceOperation,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampControlTimeout,
	)
	defer cancel()

	c.log.Info().
		Str("instance", instance.Name).
		Str("operation", string(operation)).
		Msg("Executando operação na instância AMP")

	err := amp.ControlInstance(
		ctx,
		operation,
		instance.Name,
	)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instance.Name).
			Str("operation", string(operation)).
			Msg("Operação na instância AMP falhou")

		c.updateInteractionMessageByToken(
			applicationID,
			interactionToken,
			fmt.Sprintf(
				"❌ Não foi possível concluir a operação em **%s**.\n"+
					"Consulte os logs do PalControl para mais detalhes.",
				ampInstanceDisplayName(instance),
			),
		)

		return
	}

	c.log.Info().
		Str("instance", instance.Name).
		Str("operation", string(operation)).
		Msg("Operação na instância AMP concluída")

	c.updateInteractionMessageByToken(
		applicationID,
		interactionToken,
		fmt.Sprintf(
			"%s\nInstância: `%s`",
			ampOperationSuccessMessage(
				operation,
				ampInstanceDisplayName(instance),
			),
			instance.Name,
		),
	)
}

func (c *Client) resolveAMPInstance(
	instanceName string,
) (amp.ManagedInstance, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		ampDiscoveryTimeout,
	)
	defer cancel()

	instances, err := amp.DiscoverInstances(ctx)
	if err != nil {
		return amp.ManagedInstance{}, fmt.Errorf(
			"não foi possível atualizar a lista de instâncias: %w",
			err,
		)
	}

	instanceName = strings.TrimSpace(instanceName)

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
	err := event.DeferCreateMessage(false)
	if err != nil {
		c.log.Error().
			Err(err).
			Msg("Erro adiando resposta do comando AMP")

		return false
	}

	return true
}

func buildAMPStatusMessage(
	instances []amp.ManagedInstance,
) string {
	var message strings.Builder

	message.WriteString(
		"🖥️ **PalControl — Instâncias AMP**\n\n",
	)

	if len(instances) == 0 {
		message.WriteString(
			"Nenhuma instância controlável foi encontrada.",
		)

		return message.String()
	}

	for _, instance := range instances {
		statusIcon := "⚫"
		statusText := "Desligada"

		if instance.Running {
			statusIcon = "🟢"
			statusText = "Ligada"
		}

		_, _ = fmt.Fprintf(
			&message,
			"%s **%s**\n"+
				"Estado da instância: **%s**\n"+
				"Nome: `%s`\n"+
				"Módulo: `%s`\n\n",
			statusIcon,
			ampInstanceDisplayName(instance),
			statusText,
			instance.Name,
			instance.Module,
		)
	}

	return strings.TrimSpace(
		message.String(),
	)
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
	operation amp.InstanceOperation,
) string {
	switch operation {
	case amp.InstanceOperationStart:
		return "⏳ Iniciando a instância AMP de"

	case amp.InstanceOperationStop:
		return "⏳ Parando a instância AMP de"

	case amp.InstanceOperationRestart:
		return "🔄 Reiniciando a instância AMP de"

	case amp.InstanceOperationUpdate:
		return "⬆️ Atualizando a instalação AMP de"

	default:
		return "⏳ Executando uma operação na instância AMP de"
	}
}

func ampOperationSuccessMessage(
	operation amp.InstanceOperation,
	displayName string,
) string {
	switch operation {
	case amp.InstanceOperationStart:
		return fmt.Sprintf(
			"✅ A instância AMP de **%s** foi iniciada.",
			displayName,
		)

	case amp.InstanceOperationStop:
		return fmt.Sprintf(
			"✅ A instância AMP de **%s** foi parada.",
			displayName,
		)

	case amp.InstanceOperationRestart:
		return fmt.Sprintf(
			"✅ A instância AMP de **%s** foi reiniciada.",
			displayName,
		)

	case amp.InstanceOperationUpdate:
		return fmt.Sprintf(
			"✅ A instalação AMP de **%s** foi atualizada.",
			displayName,
		)

	default:
		return fmt.Sprintf(
			"✅ A operação em **%s** foi concluída.",
			displayName,
		)
	}
}
