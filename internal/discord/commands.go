package discord

import (
	"context"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

func RegisterCommands(
	ctx context.Context,
	restClient rest.Rest,
	applicationID snowflake.ID,
) error {
	applications := rest.NewApplications(
		restClient,
	)

	_, _ = applications.SetGlobalCommands(
		applicationID,
		[]discord.ApplicationCommandCreate{},
	)

	guildID := snowflake.MustParse(
		"787371679541755935",
	)

	commands := []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "pal",
			Description: "Comandos do servidor Palworld",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionSubCommand{
					Name:        "status",
					Description: "Mostra o status dos servidores",
				},

				discord.ApplicationCommandOptionSubCommand{
					Name:        "iniciar",
					Description: "Inicia ou acorda um servidor Palworld",
					Options: []discord.ApplicationCommandOption{
						discord.ApplicationCommandOptionString{
							Name:        "servidor",
							Description: "Servidor que será iniciado",
							Required:    true,
							Choices: []discord.ApplicationCommandOptionChoiceString{
								{
									Name:  "Alamama",
									Value: "alamama",
								},
								{
									Name:  "Kalaga",
									Value: "kalaga",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := applications.SetGuildCommands(
		applicationID,
		guildID,
		commands,
	)

	return err
}
