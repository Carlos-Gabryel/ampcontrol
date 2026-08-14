package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

func (c *Client) handleAMPConfigCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !ampConfigCommandAuthorized(
		event.User().ID,
		event.Member(),
		c.ownerUserID,
		c.adminRoleIDs,
		c.allowAdministrators,
	) {
		c.log.Warn().
			Str("user_id", event.User().ID.String()).
			Msg("Comando administrativo do AmpControl recusado")
		c.sendInteractionMessage(
			event,
			"⛔ Somente o proprietário ou um administrador autorizado pode alterar as configurações das instâncias.",
		)
		return
	}

	if data.SubCommandName == nil {
		c.sendInteractionMessage(
			event,
			"⚠️ Nenhum subcomando de configuração foi informado.",
		)
		return
	}

	switch *data.SubCommandName {
	case "diagnostico", "diagnostics":
		c.handleAMPDiagnosticsCommand(event)

	case "ocultar", "hide":
		c.handleAMPInstanceVisibilityCommand(event, data, true)

	case "exibir", "show":
		c.handleAMPInstanceVisibilityCommand(event, data, false)

	case "listar", "list":
		c.handleAMPHiddenInstancesCommand(event)

	case "idle-adicionar", "idle-add":
		c.handleAMPIdleRegistrationCommand(event, data)

	case "configurar", "configure":
		c.handleAMPInstanceSettingsCommand(event, data)

	case "detalhes", "details":
		c.handleAMPInstanceSettingsDetailsCommand(event, data)

	case "restaurar", "reset":
		c.handleAMPInstanceSettingsResetCommand(event, data)

	default:
		c.sendInteractionMessage(
			event,
			"⚠️ Subcomando de configuração não reconhecido.",
		)
	}
}

func (c *Client) handleAMPInstanceSettingsCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := optStringAny(data, "servidor", "server")
	instanceName = strings.TrimSpace(instanceName)
	if !exists || instanceName == "" {
		c.updateInteractionMessage(event, "⚠️ A instância AMP não foi informada.")
		return
	}
	instance, err := c.resolveAMPInstance(instanceName)
	if err != nil {
		c.updateInteractionMessage(event, "⚠️ A instância selecionada não existe ou não pode ser controlada.")
		return
	}

	nameValue, hasName := optStringAny(data, "nome", "name")
	gameValue, hasGame := optStringAny(data, "jogo", "game")
	maximumValue, hasMaximum := optIntAny(data, "maximo", "maximum")
	addressValue, hasAddress := optStringAny(data, "endereco", "address")
	detectorValue, hasDetector := optStringAny(data, "detector")
	nameValue = strings.TrimSpace(nameValue)
	gameValue = strings.TrimSpace(gameValue)
	addressValue = strings.TrimSpace(addressValue)
	if !hasName && !hasGame && !hasMaximum && !hasAddress && !hasDetector {
		c.updateInteractionMessage(event, "⚠️ Informe pelo menos uma configuração para alterar.")
		return
	}
	if (hasName && nameValue == "") || (hasGame && gameValue == "") || (hasAddress && addressValue == "") {
		c.updateInteractionMessage(event, "⚠️ Nome e jogo não podem ficar vazios.")
		return
	}

	var previousDetection IdleDetectionSettings
	var updatedDetection IdleDetectionSettings
	detectionChanged := false
	manager := c.idleDetectionConfigurator()
	if hasDetector {
		if manager == nil {
			c.updateInteractionMessage(event, "❌ O gerenciador de detectores do Idle não está disponível.")
			return
		}
		previousDetection, err = manager.IdleDetectionSettings(instance.Name)
		if err != nil {
			c.updateInteractionMessage(event, "❌ Essa instância precisa ser adicionada ao Idle antes de configurar seu detector.")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
		updatedDetection, err = manager.SetIdleDetectionMethod(ctx, instance.Name, detectorValue)
		cancel()
		if err != nil {
			c.log.Error().Err(err).Str("instance", instance.Name).Msg("Não foi possível alterar o detector da instância")
			c.updateInteractionMessage(event, fmt.Sprintf(
				"❌ Não foi possível alterar o método de detecção de **%s**.\nErro: `%s`",
				ampInstanceDisplayName(instance), sanitizeAMPError(err),
			))
			return
		}
		detectionChanged = true
	}

	var namePointer *string
	var gamePointer *string
	var maximumPointer *int
	var addressPointer *string
	if hasName {
		namePointer = &nameValue
	}
	if hasGame {
		gamePointer = &gameValue
	}
	if hasMaximum {
		maximumPointer = &maximumValue
	}
	if hasAddress {
		addressPointer = &addressValue
	}
	setting, _ := c.instancePresentationSettings(instance.Name)
	if hasName || hasGame || hasMaximum || hasAddress {
		setting, err = c.setInstancePresentationSettings(
			instance.Name,
			namePointer,
			gamePointer,
			maximumPointer,
			addressPointer,
		)
		if err != nil {
			if detectionChanged {
				rollbackCtx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
				_, _ = manager.SetIdleDetectionMethod(rollbackCtx, instance.Name, previousDetection.Method)
				cancel()
			}
			c.updateInteractionMessage(event, "❌ Não foi possível salvar as configurações de apresentação.")
			return
		}
	}

	c.requestStatusRefresh()
	ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
	registrationErr := c.registerCommands(ctx)
	cancel()

	displayName := instance.FriendlyName
	game := instance.Game
	maximum := i18n.Choose("automático pelo servidor", "automatic from server")
	if setting.DisplayName != "" {
		displayName = setting.DisplayName
	}
	if setting.Game != "" {
		game = setting.Game
	}
	if setting.MaximumPlayers > 0 {
		maximum = fmt.Sprintf("%d", setting.MaximumPlayers)
	}
	detector := "API do AMP"
	if hasDetector {
		detector = describeIdleDetectionMethod(updatedDetection.Method)
	} else if manager != nil {
		if currentDetection, detectionErr := manager.IdleDetectionSettings(instance.Name); detectionErr == nil {
			detector = describeIdleDetectionMethod(currentDetection.Method)
		}
	}
	message := fmt.Sprintf(
		i18n.Choose("✅ Configuração de **%s** atualizada.\nNome: **%s**\nJogo: `%s`\nMáximo: `%s`\nEndereço: `%s`\nDetector: `%s`", "✅ Configuration for **%s** updated.\nName: **%s**\nGame: `%s`\nMaximum: `%s`\nAddress: `%s`\nDetector: `%s`"),
		instance.Name, displayName, game, maximum,
		dashboardInstanceAddress(setting, c.gameServerAddress), detector,
	)
	if registrationErr != nil {
		message += "\n⚠️ As preferências foram salvas, mas a lista dos comandos será atualizada na próxima conexão."
	}
	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 20*time.Second)
}

func (c *Client) handleAMPInstanceSettingsDetailsCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !c.deferAMPInteraction(event) {
		return
	}
	instanceName, exists := optStringAny(data, "servidor", "server")
	if !exists {
		c.updateInteractionMessage(event, "⚠️ A instância AMP não foi informada.")
		return
	}
	instance, err := c.resolveAMPInstance(instanceName)
	if err != nil {
		c.updateInteractionMessage(event, "⚠️ A instância selecionada não existe.")
		return
	}
	setting, customized := c.instancePresentationSettings(instance.Name)
	maximum := i18n.Choose("automático pelo servidor", "automatic from server")
	if setting.MaximumPlayers > 0 {
		maximum = fmt.Sprintf("%d", setting.MaximumPlayers)
	}
	detector := i18n.Choose("não cadastrada no Idle", "not registered in Idle")
	rcon := i18n.Choose("não configurado", "not configured")
	if manager := c.idleDetectionConfigurator(); manager != nil {
		if detection, detectionErr := manager.IdleDetectionSettings(instance.Name); detectionErr == nil {
			detector = describeIdleDetectionMethod(detection.Method)
			if detection.RCONConfigured {
				rcon = i18n.Choose("configurado", "configured")
			}
		}
	}
	c.updateInteractionMessage(event, fmt.Sprintf(
		i18n.Choose("⚙️ **Configuração de %s**\nInstância: `%s`\nJogo: `%s`\nMáximo: `%s`\nEndereço: `%s`\nDetector: `%s`\nRCON: `%s`\nPersonalização: `%t`", "⚙️ **Configuration for %s**\nInstance: `%s`\nGame: `%s`\nMaximum: `%s`\nAddress: `%s`\nDetector: `%s`\nRCON: `%s`\nCustomized: `%t`"),
		ampInstanceDisplayName(instance), instance.Name, instance.Game, maximum,
		dashboardInstanceAddress(setting, c.gameServerAddress), detector, rcon, customized,
	))
	c.deleteInteractionResponseLater(event, 30*time.Second)
}

func (c *Client) handleAMPInstanceSettingsResetCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !ampCommandConfirmed(data) {
		c.sendInteractionMessage(event, "⚠️ A restauração das configurações não foi confirmada.")
		return
	}
	if !c.deferAMPInteraction(event) {
		return
	}
	instanceName, exists := optStringAny(data, "servidor", "server")
	if !exists {
		c.updateInteractionMessage(event, "⚠️ A instância AMP não foi informada.")
		return
	}
	instance, err := c.resolveAMPInstance(instanceName)
	if err != nil {
		c.updateInteractionMessage(event, "⚠️ A instância selecionada não existe.")
		return
	}

	if manager := c.idleDetectionConfigurator(); manager != nil && c.idleInstanceRegistered(instance.Name) {
		ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
		_, err = manager.ResetIdleDetectionMethod(ctx, instance.Name)
		cancel()
		if err != nil {
			c.updateInteractionMessage(event, fmt.Sprintf("❌ Não foi possível restaurar o detector: `%s`", sanitizeAMPError(err)))
			return
		}
	}
	removed, err := c.resetInstancePresentationSettings(instance.Name)
	if err != nil {
		c.updateInteractionMessage(event, "❌ O detector foi restaurado, mas não foi possível remover a apresentação personalizada.")
		return
	}
	c.requestStatusRefresh()
	ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
	registrationErr := c.registerCommands(ctx)
	cancel()

	message := fmt.Sprintf(i18n.Choose("✅ As configurações de **%s** foram restauradas para os valores originais.", "✅ Settings for **%s** were restored to their original values."), instance.Name)
	if !removed {
		message = fmt.Sprintf(i18n.Choose("✅ O detector de **%s** foi restaurado; não havia apresentação personalizada.", "✅ The detector for **%s** was restored; there was no custom presentation."), instance.Name)
	}
	if registrationErr != nil {
		message += "\n⚠️ A lista dos comandos será atualizada na próxima conexão."
	}
	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 15*time.Second)
}

func describeIdleDetectionMethod(method string) string {
	switch method {
	case "amp_palworld_rcon":
		return "API AMP + RCON Palworld"
	case "amp_project_zomboid_rcon":
		return "API AMP + RCON Project Zomboid"
	default:
		return "API do AMP"
	}
}

func (c *Client) handleAMPIdleRegistrationCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := optStringAny(data, "servidor", "server")
	instanceName = strings.TrimSpace(instanceName)
	if !exists || instanceName == "" {
		c.updateInteractionMessage(
			event,
			"⚠️ A instância AMP não foi informada.",
		)
		return
	}

	instance, err := c.resolveAMPInstance(instanceName)
	if err != nil {
		c.updateInteractionMessage(
			event,
			"⚠️ A instância selecionada não existe ou não pode ser controlada.",
		)
		return
	}
	if c.idleInstanceRegistered(instance.Name) {
		c.updateInteractionMessage(
			event,
			fmt.Sprintf(
				i18n.Choose("ℹ️ **%s** já está cadastrada no motor de Idle.", "ℹ️ **%s** is already registered in the Idle engine."),
				ampInstanceDisplayName(instance),
			),
		)
		return
	}

	registrar := c.idleRegistrar()
	if registrar == nil {
		c.updateInteractionMessage(
			event,
			"❌ O gerenciador de cadastros do Idle não está disponível.",
		)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
	defer cancel()
	if err := registrar.RegisterIdleServer(ctx, instance); err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instance.Name).
			Msg("Não foi possível cadastrar a instância no motor de Idle")
		c.updateInteractionMessage(
			event,
			"❌ Não foi possível cadastrar a instância no motor de Idle.",
		)
		return
	}

	c.markIdleInstanceRegistered(instance.Name)
	c.setGameOverride(instance.Name, instance.Game)
	registrationErr := c.registerCommands(ctx)
	c.requestStatusRefresh()

	message := fmt.Sprintf(
		i18n.Choose("✅ **%s** foi adicionada ao Idle automático.\nDetector: `API AMP` • Limite: `15 min` • Proteção inicial: `5 min`.", "✅ **%s** was added to automatic Idle.\nDetector: `AMP API` • Limit: `15 min` • Startup protection: `5 min`."),
		ampInstanceDisplayName(instance),
	)
	if registrationErr != nil {
		c.log.Error().
			Err(registrationErr).
			Msg("Idle ativado, mas os comandos não foram atualizados")
		message += "\n⚠️ O Idle já está ativo, mas a lista do comando será atualizada na próxima conexão do bot."
	}

	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 12*time.Second)
}

func ampConfigCommandAuthorized(
	userID snowflake.ID,
	member *disgoDiscord.ResolvedMember,
	ownerUserID snowflake.ID,
	adminRoleIDs map[snowflake.ID]struct{},
	allowAdministrators bool,
) bool {
	return ampCommandPrivileged(userID, member, ownerUserID, adminRoleIDs, allowAdministrators)
}

func (c *Client) handleAMPInstanceVisibilityCommand(
	event *events.ApplicationCommandInteractionCreate,
	data disgoDiscord.SlashCommandInteractionData,
	hidden bool,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	instanceName, exists := optStringAny(data, "servidor", "server")
	instanceName = strings.TrimSpace(instanceName)
	if !exists || instanceName == "" {
		c.updateInteractionMessage(
			event,
			"⚠️ A instância AMP não foi informada.",
		)
		return
	}

	if hidden {
		instance, err := c.resolveAMPInstance(instanceName)
		if err != nil {
			c.updateInteractionMessage(
				event,
				"⚠️ A instância selecionada não existe ou não pode ser controlada.",
			)
			return
		}
		instanceName = instance.Name
	} else {
		hiddenNames := hiddenInstanceMap(c.hiddenInstanceNames())
		storedName, isHidden := hiddenNames[normalizeInstanceVisibilityKey(instanceName)]
		if !isHidden {
			c.updateInteractionMessage(
				event,
				"ℹ️ Essa instância já está visível no Discord.",
			)
			return
		}
		instanceName = storedName
	}

	changed, err := c.setInstanceHidden(instanceName, hidden)
	if err != nil {
		c.log.Error().
			Err(err).
			Str("instance", instanceName).
			Msg("Não foi possível salvar a visibilidade da instância")
		c.updateInteractionMessage(
			event,
			"❌ Não foi possível salvar a preferência de visibilidade.",
		)
		return
	}

	if !changed {
		state := i18n.Choose("visível", "visible")
		if hidden {
			state = i18n.Choose("oculta", "hidden")
		}
		c.updateInteractionMessage(
			event,
			fmt.Sprintf(i18n.Choose("ℹ️ **%s** já está %s no Discord.", "ℹ️ **%s** is already %s in Discord."), instanceName, state),
		)
		return
	}

	c.requestStatusRefresh()
	ctx, cancel := context.WithTimeout(context.Background(), ampDiscoveryTimeout)
	defer cancel()
	registrationErr := c.registerCommands(ctx)

	action := i18n.Choose("voltou a aparecer", "is visible again")
	if hidden {
		action = i18n.Choose("foi ocultada", "was hidden")
	}
	message := fmt.Sprintf(
		i18n.Choose("✅ **%s** %s no painel e nos comandos do Discord.", "✅ **%s** %s in the Discord dashboard and commands."),
		instanceName,
		action,
	)
	if registrationErr != nil {
		c.log.Error().
			Err(registrationErr).
			Msg("Preferência salva, mas os comandos não foram atualizados")
		message += "\n⚠️ A preferência foi salva, mas a lista dos comandos só será atualizada na próxima conexão do bot."
	}

	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 8*time.Second)
}

func (c *Client) handleAMPHiddenInstancesCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	hiddenNames := c.hiddenInstanceNames()
	message := "ℹ️ Nenhuma instância está oculta no Discord."
	if len(hiddenNames) > 0 {
		message = i18n.Choose("**Instâncias ocultas:**\n- `", "**Hidden instances:**\n- `") +
			strings.Join(hiddenNames, "`\n- `") + "`"
	}

	c.updateInteractionMessage(event, message)
	c.deleteInteractionResponseLater(event, 12*time.Second)
}
