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
}

func (c *Client) handleAMPInteractionEvent(
	event *events.ApplicationCommandInteractionCreate,
) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c.log.Error().
				Interface("panic", recovered).
				Msg("Panic processando comando AMP")
		}
	}()

	data := event.SlashCommandInteractionData()

	if data.CommandName() != "amp" &&
		data.CommandName() != "ampconfig" {
		return
	}

	channelID := event.Channel().ID()
	if !ampCommandChannelAllowed(
		channelID,
		c.notificationChannelID,
	) {
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
) bool {
	return channelID != 0 && channelID == allowedChannelID
}
