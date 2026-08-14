package discord

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"
)

const (
	maximumDiscordChoices = 25
	maximumChoiceNameSize = 100
)

func RegisterCommands(
	restClient rest.Rest,
	applicationID snowflake.ID,
	guildID snowflake.ID,
	instances []amp.ManagedInstance,
	gameOverrides map[string]string,
	presentationOverrides []instancePresentationOverride,
	hiddenInstanceNames []string,
	idleRegisteredNames []string,
) ([]amp.ManagedInstance, error) {
	applications := rest.NewApplications(
		restClient,
	)

	_, _ = applications.SetGlobalCommands(
		applicationID,
		[]discord.ApplicationCommandCreate{},
	)

	applyCommandGameNames(instances, gameOverrides)
	applyInstancePresentationOverrides(instances, presentationOverrides)
	visibleInstances, hiddenInstances := splitAMPInstancesByVisibility(
		instances,
		hiddenInstanceNames,
	)

	instanceChoices, err := buildAMPInstanceChoices(
		visibleInstances,
	)
	if err != nil {
		return nil, err
	}
	hiddenInstanceChoices, err := buildAMPInstanceChoices(hiddenInstances)
	if err != nil {
		return nil, err
	}
	idleCandidateChoices, err := buildAMPInstanceChoices(
		filterUnregisteredAMPInstances(instances, idleRegisteredNames),
	)
	if err != nil {
		return nil, err
	}

	commands := []discord.ApplicationCommandCreate{
		buildAMPCommand(instanceChoices),
		buildAMPConfigCommand(
			instanceChoices,
			hiddenInstanceChoices,
			idleCandidateChoices,
		),
	}

	_, err = applications.SetGuildCommands(
		applicationID,
		guildID,
		commands,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível registrar os comandos da guilda: %w",
			err,
		)
	}

	return instances, nil
}

func (c *Client) registerCommands(ctx context.Context) error {
	instances, err := c.discoverAMPInstances(ctx)
	if err != nil {
		return fmt.Errorf("não foi possível descobrir as instâncias para registrar os comandos: %w", err)
	}
	return c.registerCommandsWithInstances(instances)
}

func (c *Client) registerCommandsWithInstances(instances []amp.ManagedInstance) error {
	c.commandRegistrationMu.Lock()
	defer c.commandRegistrationMu.Unlock()

	instances, err := RegisterCommands(
		c.bot.Rest,
		c.bot.ApplicationID,
		c.guildID,
		instances,
		c.gameOverridesSnapshot(),
		c.instancePresentationSettingsSnapshot(),
		c.hiddenInstanceNames(),
		c.idleRegisteredInstanceNames(),
	)
	if err != nil {
		return err
	}

	c.commandInventoryMu.Lock()
	c.commandInventory = ampInstanceInventorySignature(instances)
	c.commandInventoryMu.Unlock()
	return nil
}

func ampInstanceInventorySignature(instances []amp.ManagedInstance) string {
	names := make([]string, 0, len(instances))
	for _, instance := range instances {
		name := normalizeInstanceVisibilityKey(instance.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, "\n")
}

func (c *Client) refreshCommandsForInventory(
	ctx context.Context,
	instances []amp.ManagedInstance,
) {
	if c == nil || c.bot == nil || c.bot.ApplicationID == 0 {
		return
	}

	signature := ampInstanceInventorySignature(instances)
	c.commandInventoryMu.RLock()
	current := c.commandInventory
	c.commandInventoryMu.RUnlock()
	if signature == current {
		return
	}

	if err := c.registerCommandsWithInstances(instances); err != nil {
		c.log.Warn().
			Err(err).
			Msg("Inventário AMP mudou, mas os comandos não puderam ser atualizados")
		return
	}

	c.log.Info().
		Int("instances", len(instances)).
		Msg("Comandos atualizados após mudança no inventário AMP")
}

func buildAMPCommand(
	instanceChoices []discord.ApplicationCommandOptionChoiceString,
) discord.SlashCommandCreate {
	l := i18n.Choose
	return discord.SlashCommandCreate{
		Name:        "amp",
		Description: l("Controla os jogos e as instâncias do AMP", "Controls AMP game servers and instances"),
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "status",
				Description: l("Mostra os estados Offline, Idle e Online", "Shows Offline, Idle, and Online states"),
			},
			buildAMPControlSubCommand(
				l("iniciar", "start"),
				l("Inicia a instância e o processo do jogo", "Starts the instance and game process"),
				l("Servidor que será iniciado", "Server to start"),
				instanceChoices,
			),
			buildAMPControlSubCommand(
				l("parar", "stop"),
				l("Para somente o jogo e mantém a instância em Idle", "Stops only the game and keeps the instance Idle"),
				l("Servidor que será colocado em Idle", "Server to place in Idle"),
				instanceChoices,
			),
			buildAMPControlSubCommand(
				l("reiniciar", "restart"),
				l("Reinicia somente o processo do jogo", "Restarts only the game process"),
				l("Jogo que será reiniciado", "Game server to restart"),
				instanceChoices,
			),
			buildAMPConfirmedControlSubCommand(
				l("desligar", "shutdown"),
				l("Desliga completamente uma instância AMP", "Shuts down an AMP instance completely"),
				l("Instância AMP que será desligada", "AMP instance to shut down"),
				l("Confirma o desligamento completo da instância", "Confirms the complete instance shutdown"),
				l("Sim, desligar completamente", "Yes, shut down completely"),
				instanceChoices,
			),
			buildAMPConfirmedControlSubCommand(
				l("atualizar", "update"),
				l("Atualiza somente a instalação AMP da instância", "Updates the selected AMP instance"),
				l("Instância AMP que será atualizada", "AMP instance to update"),
				l("Confirma a atualização da instalação AMP", "Confirms the AMP instance update"),
				l("Sim, atualizar a instalação AMP", "Yes, update the AMP instance"),
				instanceChoices,
			),
		},
	}
}

func buildAMPConfigCommand(
	visibleChoices []discord.ApplicationCommandOptionChoiceString,
	hiddenChoices []discord.ApplicationCommandOptionChoiceString,
	idleCandidateChoices []discord.ApplicationCommandOptionChoiceString,
) discord.SlashCommandCreate {
	l := i18n.Choose
	configurationChoices := append(
		append([]discord.ApplicationCommandOptionChoiceString(nil), visibleChoices...),
		hiddenChoices...,
	)
	minimumTextLength := 1
	maximumTextLength := 80
	maximumAddressLength := 120
	minimumPlayers := 1
	maximumPlayers := 100000
	return discord.SlashCommandCreate{
		Name:        "ampconfig",
		Description: l("Configura a apresentação das instâncias do AmpControl", "Configures AmpControl instance presentation"),
		DefaultMemberPermissions: omit.NewPtr(
			discord.PermissionAdministrator,
		),
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        l("diagnostico", "diagnostics"),
				Description: l("Verifica a saúde do AmpControl e das integrações", "Checks AmpControl and integration health"),
			},
			buildAMPControlSubCommand(
				l("ocultar", "hide"),
				l("Oculta uma instância do painel e dos comandos", "Hides an instance from the dashboard and commands"),
				l("Instância que deixará de aparecer no Discord", "Instance to hide from Discord"),
				visibleChoices,
			),
			discord.ApplicationCommandOptionSubCommand{
				Name:        l("configurar", "configure"),
				Description: l("Altera nome, jogo, endereço, limite ou detector", "Changes name, game, address, limit, or detector"),
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{
						Name:        l("servidor", "server"),
						Description: l("Instância que será configurada", "Instance to configure"),
						Required:    true,
						Choices:     configurationChoices,
					},
					discord.ApplicationCommandOptionString{
						Name:        l("nome", "name"),
						Description: l("Novo nome exibido no Discord", "New name displayed in Discord"),
						MinLength:   &minimumTextLength,
						MaxLength:   &maximumTextLength,
					},
					discord.ApplicationCommandOptionString{
						Name:        l("jogo", "game"),
						Description: l("Nome correto do jogo", "Correct game name"),
						MinLength:   &minimumTextLength,
						MaxLength:   &maximumTextLength,
					},
					discord.ApplicationCommandOptionInt{
						Name:        l("maximo", "maximum"),
						Description: l("Quantidade máxima de jogadores exibida no painel", "Maximum player count displayed on the dashboard"),
						MinValue:    &minimumPlayers,
						MaxValue:    &maximumPlayers,
					},
					discord.ApplicationCommandOptionString{
						Name:        l("endereco", "address"),
						Description: l("IP, domínio e porta usados para entrar no servidor", "IP, domain, and port used to join the server"),
						MinLength:   &minimumTextLength,
						MaxLength:   &maximumAddressLength,
					},
					discord.ApplicationCommandOptionString{
						Name:        "detector",
						Description: l("Método usado para confirmar jogadores conectados", "Method used to confirm connected players"),
						Choices: []discord.ApplicationCommandOptionChoiceString{
							{Name: "API do AMP", Value: "amp"},
							{Name: "API AMP + RCON Palworld", Value: "amp_palworld_rcon"},
							{Name: "API AMP + RCON Project Zomboid", Value: "amp_project_zomboid_rcon"},
						},
					},
				},
			},
			buildAMPControlSubCommand(
				l("detalhes", "details"),
				l("Mostra a configuração atual de uma instância", "Shows an instance's current configuration"),
				l("Instância que será consultada", "Instance to inspect"),
				configurationChoices,
			),
			buildAMPConfirmedControlSubCommand(
				l("restaurar", "reset"),
				l("Remove personalizações e restaura os valores originais", "Removes customizations and restores original values"),
				l("Instância que voltará à configuração original", "Instance to restore"),
				l("Confirma a remoção das personalizações", "Confirms removal of customizations"),
				l("Sim, restaurar a configuração original", "Yes, restore the original configuration"),
				configurationChoices,
			),
			buildAMPControlSubCommand(
				l("exibir", "show"),
				l("Volta a exibir uma instância no painel e nos comandos", "Shows an instance in the dashboard and commands again"),
				l("Instância que voltará a aparecer no Discord", "Instance to show in Discord"),
				hiddenChoices,
			),
			buildAMPControlSubCommand(
				l("idle-adicionar", "idle-add"),
				l("Adiciona uma instância ao Idle automático", "Adds an instance to automatic Idle"),
				l("Instância que usará Idle ativo após 15 minutos", "Instance to use active Idle after 15 minutes"),
				idleCandidateChoices,
			),
			discord.ApplicationCommandOptionSubCommand{
				Name:        l("listar", "list"),
				Description: l("Lista as instâncias ocultas do Discord", "Lists instances hidden from Discord"),
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
				Name:        i18n.Choose("servidor", "server"),
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
				Name:        i18n.Choose("servidor", "server"),
				Description: optionDescription,
				Required:    true,
				Choices:     instanceChoices,
			},
			discord.ApplicationCommandOptionString{
				Name:        i18n.Choose("confirmar", "confirm"),
				Description: confirmationDescription,
				Required:    true,
				Choices: []discord.ApplicationCommandOptionChoiceString{
					{
						Name:  confirmationChoiceName,
						Value: i18n.Choose("sim", "yes"),
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
		game = i18n.Choose("Jogo desconhecido", "Unknown game")
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
