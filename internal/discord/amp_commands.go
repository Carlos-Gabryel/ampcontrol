package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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

	case "iniciar", "start":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationStart,
		)

	case "parar", "stop":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationStop,
		)

	case "reiniciar", "restart":
		c.handleAMPControlCommand(
			event,
			data,
			ampCommandOperationRestart,
		)

	case "desligar", "shutdown":
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

	case "atualizar", "update":
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

func ampCommandPrivileged(
	userID snowflake.ID,
	member *disgoDiscord.ResolvedMember,
	ownerUserID snowflake.ID,
	adminRoleIDs map[snowflake.ID]struct{},
	allowAdministrators bool,
) bool {
	if ownerUserID != 0 && userID == ownerUserID {
		return true
	}
	if member == nil {
		return false
	}
	for _, roleID := range member.RoleIDs {
		if _, allowed := adminRoleIDs[roleID]; allowed {
			return true
		}
	}
	return allowAdministrators && ampCommandAuthorized(member)
}

func ampCommandConfirmed(
	data disgoDiscord.SlashCommandInteractionData,
) bool {
	confirmation, exists := optStringAny(data, "confirmar", "confirm")

	return exists && (confirmation == "sim" || confirmation == "yes")
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

func (c *Client) handleAMPControlCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	operation ampCommandOperation,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := optStringAny(data, "servidor", "server")
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
		ampCommandBypassesPlayerProtection(
			event.User().ID,
			event.Member(),
			c.ownerUserID,
			c.adminRoleIDs,
			c.allowAdministrators,
		),
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
		c.finishCommandAudit(
			event.Token(),
			commandAuditPhaseFailed,
			i18n.Choose("Não foi possível iniciar a resposta do comando no Discord.", "Could not start the command response in Discord."),
			"",
		)

		return false
	}

	return true
}
