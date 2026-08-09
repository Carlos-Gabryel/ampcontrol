package discord

import (
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
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

	if data.CommandName() != "amp" {
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

	c.handleAMPCommand(
		event,
		data,
	)
}
