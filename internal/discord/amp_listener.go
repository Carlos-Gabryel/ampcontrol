package discord

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

func (c *Client) EnableAMPCommandHandling() {
	c.bot.AddEventListeners(
		bot.NewListenerFunc(
			func(
				event *events.ApplicationCommandInteractionCreate,
			) {
				c.handleAMPInteractionEvent(
					event,
				)
			},
		),
	)
	c.bot.AddEventListeners(
		bot.NewListenerFunc(
			func(event *events.ComponentInteractionCreate) {
				c.handleDashboardComponent(event)
			},
		),
	)
}

func (c *Client) handleAMPInteractionEvent(
	event *events.ApplicationCommandInteractionCreate,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.log.Error().
				Interface("panic", recovered).
				Msg("Panic processando comando AMP")
			c.finishCommandAudit(
				event.Token(),
				commandAuditPhaseFailed,
				"O comando foi interrompido por uma falha interna.",
				"Não concluído",
			)
		}
	}()

	data := event.SlashCommandInteractionData()

	if data.CommandName() != "amp" &&
		data.CommandName() != "ampconfig" {
		return
	}

	channelID := event.Channel().ID()
	channelAllowed := ampCommandChannelAllowed(
		channelID,
		c.notificationChannelID,
		c.restrictCommandChannel,
	)

	auditAccepted, auditReason := c.commandAuditDecision(
		event,
		data,
		channelAllowed,
	)
	var cooldownDecision ampCommandCooldownDecision
	if auditAccepted && data.CommandName() == "amp" {
		cooldownDecision = c.reserveAMPCommandCooldown(
			event.User().ID,
			data,
		)
		if !cooldownDecision.Allowed {
			auditAccepted = false
			auditReason = cooldownDecision.AuditReason()
		}
	}
	c.beginCommandAudit(
		event.Token(),
		newCommandAuditRecord(
			event,
			data,
			auditAccepted,
			auditReason,
		),
	)

	if !channelAllowed {
		c.log.Warn().
			Str("channel_id", channelID.String()).
			Msg("Comando AMP recusado fora do canal autorizado")

		c.sendInteractionMessage(
			event,
			"⛔ O AmpControl só aceita comandos no canal <#"+
				c.notificationChannelID.String()+">.",
		)

		return
	}

	if data.CommandName() == "amp" && !cooldownDecision.Allowed {
		c.log.Warn().
			Str("user_id", event.User().ID.String()).
			Str("server", cooldownDecision.Server).
			Dur("remaining", cooldownDecision.Remaining).
			Str("scope", string(cooldownDecision.Scope)).
			Msg("Comando AMP recusado por cooldown")

		c.sendInteractionMessage(
			event,
			cooldownDecision.UserMessage(),
		)

		return
	}

	c.log.Info().
		Str("command", data.CommandName()).
		Str("command_path", data.CommandPath()).
		Interface(
			"subcommand",
			data.SubCommandName,
		).
		Interface(
			"options",
			data.Options,
		).
		Msg("Comando AMP recebido")

	if data.CommandName() == "ampconfig" {
		c.handleAMPConfigCommand(event, data)
		return
	}

	c.handleAMPCommand(event, data)
}

func ampCommandChannelAllowed(
	channelID snowflake.ID,
	allowedChannelID snowflake.ID,
	restricted bool,
) bool {
	if !restricted {
		return channelID != 0
	}
	return channelID != 0 && channelID == allowedChannelID
}
