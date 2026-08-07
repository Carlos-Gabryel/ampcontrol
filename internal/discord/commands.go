package discord

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const (
	discordGuildID        = "787371679541755935"
	maximumDiscordChoices = 25
	maximumChoiceNameSize = 100
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

	instances, err := amp.DiscoverInstances(ctx)
	if err != nil {
		return fmt.Errorf(
			"não foi possível descobrir as instâncias para registrar os comandos: %w",
			err,
		)
	}

	instanceChoices, err := buildAMPInstanceChoices(
		instances,
	)
	if err != nil {
		return err
	}

	commands := []discord.ApplicationCommandCreate{
		buildPalCommand(),
		buildAMPCommand(instanceChoices),
	}

	guildID := snowflake.MustParse(
		discordGuildID,
	)

	_, err = applications.SetGuildCommands(
		applicationID,
		guildID,
		commands,
	)
	if err != nil {
		return fmt.Errorf(
			"não foi possível registrar os comandos da guilda: %w",
			err,
		)
	}

	return nil
}

func buildPalCommand() discord.SlashCommandCreate {
	return discord.SlashCommandCreate{
		Name:        "pal",
		Description: "Comandos dos servidores Palworld",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "status",
				Description: "Mostra o status dos servidores Palworld",
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "iniciar",
				Description: "Inicia ou acorda um servidor Palworld",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        "servidor",
						Description: "Servidor Palworld que será iniciado",
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
	}
}

func buildAMPCommand(
	instanceChoices []discord.ApplicationCommandOptionChoiceString,
) discord.SlashCommandCreate {
	return discord.SlashCommandCreate{
		Name:        "amp",
		Description: "Controla os jogos e as instâncias do AMP",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "status",
				Description: "Mostra os estados Offline, Idle e Online",
			},
			buildAMPControlSubCommand(
				"iniciar",
				"Inicia a instância e o processo do jogo",
				"Servidor que será iniciado",
				instanceChoices,
			),
			buildAMPControlSubCommand(
				"parar",
				"Para somente o jogo e mantém a instância em Idle",
				"Servidor que será colocado em Idle",
				instanceChoices,
			),
			buildAMPControlSubCommand(
				"reiniciar",
				"Reinicia somente o processo do jogo",
				"Jogo que será reiniciado",
				instanceChoices,
			),
			buildAMPConfirmedControlSubCommand(
				"desligar",
				"Desliga completamente uma instância AMP",
				"Instância AMP que será desligada",
				"Confirma o desligamento completo da instância",
				"Sim, desligar completamente",
				instanceChoices,
			),
			buildAMPConfirmedControlSubCommand(
				"atualizar",
				"Atualiza somente a instalação AMP da instância",
				"Instância AMP que será atualizada",
				"Confirma a atualização da instalação AMP",
				"Sim, atualizar a instalação AMP",
				instanceChoices,
			),
		},
	}
}

func buildAMPControlSubCommand(
	name string,
	description string,
	optionDescription string,
	instanceChoices []discord.ApplicationCommandOptionChoiceString,
) discord.ApplicationCommandOptionSubCommand {
	return discord.ApplicationCommandOptionSubCommand{
		Name:        name,
		Description: description,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "servidor",
				Description: optionDescription,
				Required:    true,
				Choices:     instanceChoices,
			},
		},
	}
}

func buildAMPConfirmedControlSubCommand(
	name string,
	description string,
	optionDescription string,
	confirmationDescription string,
	confirmationChoiceName string,
	instanceChoices []discord.ApplicationCommandOptionChoiceString,
) discord.ApplicationCommandOptionSubCommand {
	return discord.ApplicationCommandOptionSubCommand{
		Name:        name,
		Description: description,
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionString{
				Name:        "servidor",
				Description: optionDescription,
				Required:    true,
				Choices:     instanceChoices,
			},
			discord.ApplicationCommandOptionString{
				Name:        "confirmar",
				Description: confirmationDescription,
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{
						Name:  confirmationChoiceName,
						Value: "sim",
					},
				},
			},
		},
	}
}

func buildAMPInstanceChoices(
	instances []amp.ManagedInstance,
) ([]discord.ApplicationCommandOptionChoiceString, error) {
	if len(instances) == 0 {
		return nil, fmt.Errorf(
			"nenhuma instância AMP controlável foi encontrada",
		)
	}

	if len(instances) > maximumDiscordChoices {
		return nil, fmt.Errorf(
			"foram encontradas %d instâncias, mas o Discord aceita no máximo %d opções estáticas",
			len(instances),
			maximumDiscordChoices,
		)
	}

	choices := make(
		[]discord.ApplicationCommandOptionChoiceString,
		0,
		len(instances),
	)

	for _, instance := range instances {
		instanceName := strings.TrimSpace(
			instance.Name,
		)

		if instanceName == "" {
			return nil, fmt.Errorf(
				"foi encontrada uma instância AMP sem nome",
			)
		}

		choices = append(
			choices,
			discord.ApplicationCommandOptionChoiceString{
				Name: buildAMPChoiceName(
					instance,
				),
				Value: instanceName,
			},
		)
	}

	return choices, nil
}

func buildAMPChoiceName(
	instance amp.ManagedInstance,
) string {
	friendlyName := strings.TrimSpace(
		instance.FriendlyName,
	)

	instanceName := strings.TrimSpace(
		instance.Name,
	)

	name := instanceName

	if friendlyName != "" &&
		!strings.EqualFold(friendlyName, instanceName) {
		name = fmt.Sprintf(
			"%s — %s",
			friendlyName,
			instanceName,
		)
	}

	if utf8.RuneCountInString(name) <= maximumChoiceNameSize {
		return name
	}

	runes := []rune(name)

	return string(
		runes[:maximumChoiceNameSize],
	)
}
