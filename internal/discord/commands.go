package discord

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
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
	gameOverrides map[string]string,
	hiddenInstanceNames []string,
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

	applyCommandGameNames(instances, gameOverrides)
	visibleInstances, hiddenInstances := splitAMPInstancesByVisibility(
		instances,
		hiddenInstanceNames,
	)

	instanceChoices, err := buildAMPInstanceChoices(
		visibleInstances,
	)
	if err != nil {
		return err
	}
	hiddenInstanceChoices, err := buildAMPInstanceChoices(hiddenInstances)
	if err != nil {
		return err
	}

	commands := []discord.ApplicationCommandCreate{
		buildAMPCommand(instanceChoices),
		buildAMPConfigCommand(instanceChoices, hiddenInstanceChoices),
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

func (c *Client) registerCommands(ctx context.Context) error {
	c.commandRegistrationMu.Lock()
	defer c.commandRegistrationMu.Unlock()

	return RegisterCommands(
		ctx,
		c.bot.Rest,
		c.bot.ApplicationID,
		c.gameOverrides,
		c.hiddenInstanceNames(),
	)
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

func buildAMPConfigCommand(
	visibleChoices []discord.ApplicationCommandOptionChoiceString,
	hiddenChoices []discord.ApplicationCommandOptionChoiceString,
) discord.SlashCommandCreate {
	return discord.SlashCommandCreate{
		Name:        "ampconfig",
		Description: "Configura a apresentação das instâncias do AmpControl",
		DefaultMemberPermissions: omit.NewPtr(
			discord.PermissionAdministrator,
		),
		Options: []discord.ApplicationCommandOption{
			buildAMPControlSubCommand(
				"ocultar",
				"Oculta uma instância do painel e dos comandos",
				"Instância que deixará de aparecer no Discord",
				visibleChoices,
			),
			buildAMPControlSubCommand(
				"exibir",
				"Volta a exibir uma instância no painel e nos comandos",
				"Instância que voltará a aparecer no Discord",
				hiddenChoices,
			),
			discord.ApplicationCommandOptionSubCommand{
				Name:        "listar",
				Description: "Lista as instâncias ocultas do Discord",
			},
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

	if friendlyName == "" {
		friendlyName = strings.TrimSpace(instance.Name)
	}

	game := strings.TrimSpace(instance.Game)
	if game == "" {
		game = strings.TrimSpace(instance.Module)
	}
	if game == "" {
		game = "Jogo desconhecido"
	}

	name := fmt.Sprintf("%s — %s", friendlyName, game)

	if utf8.RuneCountInString(name) <= maximumChoiceNameSize {
		return name
	}

	runes := []rune(name)

	return string(
		runes[:maximumChoiceNameSize],
	)
}

func applyCommandGameNames(
	instances []amp.ManagedInstance,
	gameOverrides map[string]string,
) {
	for index := range instances {
		for instanceName, game := range gameOverrides {
			if strings.EqualFold(instances[index].Name, instanceName) &&
				strings.TrimSpace(game) != "" {
				instances[index].Game = strings.TrimSpace(game)
				break
			}
		}

		if strings.TrimSpace(instances[index].Game) == "" {
			instances[index].Game = strings.TrimSpace(instances[index].Module)
		}
	}
}
