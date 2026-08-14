package discord

import (
	"bytes"
	"embed"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

const (
	dashboardServersPerMessage = 1
	dashboardComponentPrefix   = "ampdash"
)

//go:embed game-logos/*.png
var gameLogoAssets embed.FS

func buildAMPStatusPages(statuses []ampInstanceStatusView, updatedAt time.Time, defaultAddress string) [][]disgoDiscord.LayoutComponent {
	if len(statuses) == 0 {
		return [][]disgoDiscord.LayoutComponent{{
			disgoDiscord.NewContainer(disgoDiscord.NewTextDisplay(i18n.Choose("Nenhuma instância controlável foi encontrada.", "No controllable instance was found."))).WithAccentColor(0x5865F2),
		}}
	}

	pages := make([][]disgoDiscord.LayoutComponent, 0, (len(statuses)+dashboardServersPerMessage-1)/dashboardServersPerMessage)
	for start := 0; start < len(statuses); start += dashboardServersPerMessage {
		end := min(start+dashboardServersPerMessage, len(statuses))
		components := make([]disgoDiscord.LayoutComponent, 0, end-start+1)
		for _, status := range statuses[start:end] {
			components = append(components, buildAMPServerContainer(status, defaultAddress))
		}
		footer := fmt.Sprintf(i18n.Choose("Atualizado <t:%d:R>", "Updated <t:%d:R>"), updatedAt.Unix())
		if end == len(statuses) {
			footer = "🟢 Online  •  🟡 Idle  •  🔴 Offline\n" + footer
		}
		components = append(components, disgoDiscord.NewTextDisplay(footer))
		pages = append(pages, components)
	}
	return pages
}

func buildAMPServerContainer(status ampInstanceStatusView, defaultAddress string) disgoDiscord.ContainerComponent {
	_, state := describeAMPInstanceStatus(status)
	game := dashboardGameName(status.Instance)
	uptime := "0 min"
	if status.ApplicationStatus != nil {
		uptime = formatAMPUptime(status.ApplicationStatus.Uptime)
	}
	players := dashboardPlayerCount(status)
	cpu, memory := dashboardResourceUsage(status.ApplicationStatus)
	address := dashboardInstanceAddress(instancePresentationOverride{Address: status.Address}, defaultAddress)
	name := ampInstanceDisplayName(status.Instance)

	headerText := disgoDiscord.NewTextDisplay(fmt.Sprintf(
		"## %s\n### %s",
		name, game,
	))
	var header disgoDiscord.ContainerSubComponent = headerText
	if imageURL := gameIconURL(game); imageURL != "" {
		header = disgoDiscord.NewSection(headerText).
			WithAccessory(disgoDiscord.NewThumbnail(imageURL).WithDescription(i18n.Choose("Logo de ", "Logo for ") + game))
	}

	statusBlock := disgoDiscord.NewTextDisplay(fmt.Sprintf(
		i18n.Choose("### Status do servidor\n**%s**", "### Server status\n**%s**"),
		state,
	))
	details := disgoDiscord.NewTextDisplay(fmt.Sprintf(
		i18n.Choose("### Endereço do servidor\n`%s`\n\n### Desempenho\n**CPU**　　**Memória**　　**Tempo online**\n`%s`　　`%s`　　`%s`\n\n### Jogadores\n`%s`", "### Server address\n`%s`\n\n### Performance\n**CPU**　　**Memory**　　**Uptime**\n`%s`　　`%s`　　`%s`\n\n### Players\n`%s`"),
		address, cpu, memory, uptime, players,
	))

	phase := amp.ApplicationPhaseUnknown
	if status.ApplicationStatus != nil {
		phase = status.ApplicationStatus.Phase()
	}
	startDisabled := status.Instance.Running && phase != amp.ApplicationPhaseIdle
	stopDisabled := !status.Instance.Running || phase != amp.ApplicationPhaseOnline
	restartDisabled := stopDisabled

	row := disgoDiscord.NewActionRow(
		disgoDiscord.NewSuccessButton(i18n.Choose("Iniciar", "Start"), dashboardComponentID("start", status.Instance.Name)).WithDisabled(startDisabled),
		disgoDiscord.NewDangerButton(i18n.Choose("Parar", "Stop"), dashboardComponentID("stop", status.Instance.Name)).WithDisabled(stopDisabled),
		disgoDiscord.NewSecondaryButton(i18n.Choose("Reiniciar", "Restart"), dashboardComponentID("restart", status.Instance.Name)).WithDisabled(restartDisabled),
		disgoDiscord.NewPrimaryButton(i18n.Choose("Atualizar", "Update"), dashboardComponentID("update", status.Instance.Name)),
		disgoDiscord.NewSecondaryButton(i18n.Choose("Detalhes", "Details"), dashboardComponentID("details", status.Instance.Name)),
	)

	return disgoDiscord.NewContainer(
		header,
		disgoDiscord.NewLargeSeparator(),
		statusBlock,
		disgoDiscord.NewSmallSeparator(),
		details,
		disgoDiscord.NewLargeSeparator(),
		row,
	).WithAccentColor(dashboardStatusColor(status))
}

func dashboardComponentID(action, instance string) string {
	return dashboardComponentPrefix + ":" + action + ":" + instance
}

func dashboardPlayerCount(status ampInstanceStatusView) string {
	if status.PlayerCounts != nil {
		return fmt.Sprintf("%d/%d", status.PlayerCounts.Current, status.PlayerCounts.Maximum)
	}
	if status.PlayerMaxOverride > 0 {
		current := "?"
		if !status.Instance.Running {
			current = "0"
		}
		return fmt.Sprintf("%s/%d", current, status.PlayerMaxOverride)
	}
	if !status.Instance.Running {
		return "0/?"
	}
	return "?/?"
}

func dashboardResourceUsage(status *amp.ApplicationStatus) (string, string) {
	if status == nil {
		return i18n.Choose("indisponível", "unavailable"), i18n.Choose("indisponível", "unavailable")
	}
	return dashboardMetric(status.Metrics, "cpu"), dashboardMetric(status.Metrics, "memory", "memória", "memoria")
}

func dashboardMetric(metrics map[string]amp.StatusMetric, names ...string) string {
	for metricName, metric := range metrics {
		lower := strings.ToLower(strings.TrimSpace(metricName))
		for _, name := range names {
			if strings.Contains(lower, name) {
				unit := strings.TrimSpace(metric.Units)
				if unit == "" && strings.Contains(lower, "cpu") {
					unit = "%"
				}
				return fmt.Sprintf("%.1f%s", metric.RawValue, unit)
			}
		}
	}
	return i18n.Choose("indisponível", "unavailable")
}

func dashboardStatusColor(status ampInstanceStatusView) int {
	if !status.Instance.Running || status.ApplicationError != nil || status.ApplicationStatus == nil {
		return 0xED4245
	}
	if status.ApplicationStatus.Phase() == amp.ApplicationPhaseIdle {
		return 0xFEE75C
	}
	if status.ApplicationStatus.Phase() == amp.ApplicationPhaseOnline {
		return 0x57F287
	}
	return 0x5865F2
}

func dashboardInstanceAddress(setting instancePresentationOverride, fallback string) string {
	if address := strings.TrimSpace(setting.Address); address != "" {
		return address
	}
	if fallback := strings.TrimSpace(fallback); fallback != "" {
		return fallback
	}
	return i18n.Choose("não configurado", "not configured")
}

func (c *Client) handleDashboardComponent(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 || parts[0] != dashboardComponentPrefix {
		return
	}
	if event.Channel().ID() != c.notificationChannelID {
		_ = event.CreateMessage(disgoDiscord.NewMessageCreate().WithContent(i18n.Text("⛔ Este painel só funciona no canal do AmpControl.")).WithEphemeral(true))
		return
	}
	if err := event.DeferCreateMessage(true); err != nil {
		return
	}
	action, instanceName := parts[1], parts[2]
	instance, err := c.resolveAMPInstance(instanceName)
	if err != nil || c.instanceHidden(instanceName) {
		c.updateInteractionMessageByToken(event.ApplicationID(), event.Token(), "⚠️ A instância não está disponível.")
		return
	}
	if action == "details" {
		c.handleDashboardDetails(event, instance)
		return
	}
	operation, valid := dashboardOperation(action)
	if !valid {
		return
	}

	decision := c.commandCooldowns.reserve(event.User().ID, instance.Name)
	record := commandAuditRecord{
		UserID: event.User().ID, UserName: event.User().EffectiveName(), ChannelID: event.Channel().ID(),
		Command: "/painel " + action, Server: instance.Name, Accepted: decision.Allowed,
		Reason: "Recebido pelo painel", CreatedAt: time.Now(),
	}
	if !decision.Allowed {
		record.Reason = decision.AuditReason()
		c.beginCommandAudit(event.Token(), record)
		c.updateInteractionMessageByToken(event.ApplicationID(), event.Token(), decision.UserMessage())
		return
	}
	c.beginCommandAudit(event.Token(), record)
	c.updateInteractionMessageByToken(event.ApplicationID(), event.Token(), fmt.Sprintf(
		"%s **%s**\nInstância: `%s`", ampOperationProgressMessage(operation), ampInstanceDisplayName(instance), instance.Name,
	))
	go c.executeAMPControlOperation(
		event.ApplicationID(), event.Token(), instance, operation,
		ampCommandBypassesPlayerProtection(
			event.User().ID,
			event.Member(),
			c.ownerUserID,
			c.adminRoleIDs,
			c.allowAdministrators,
		),
	)
}

func dashboardOperation(action string) (ampCommandOperation, bool) {
	switch action {
	case "start":
		return ampCommandOperationStart, true
	case "stop":
		return ampCommandOperationStop, true
	case "restart":
		return ampCommandOperationRestart, true
	case "update":
		return ampCommandOperationUpdate, true
	default:
		return "", false
	}
}

func (c *Client) handleDashboardDetails(event *events.ComponentInteractionCreate, instance amp.ManagedInstance) {
	setting, _ := c.instancePresentationSettings(instance.Name)
	status := collectSingleAMPStatus(c, instance)
	icon, state := describeAMPInstanceStatus(status)
	uptime := "0 min"
	if status.ApplicationStatus != nil {
		uptime = formatAMPUptime(status.ApplicationStatus.Uptime)
	}
	cpu, memory := dashboardResourceUsage(status.ApplicationStatus)
	embed := disgoDiscord.NewEmbed().
		WithAuthor(ampInstanceDisplayName(instance), "", "").
		AddField("Status", icon+" "+state, true).
		AddField(i18n.Choose("Jogo", "Game"), instance.Game, true).
		AddField(i18n.Choose("Endereço", "Address"), dashboardInstanceAddress(setting, c.gameServerAddress), false).
		AddField("CPU", cpu, true).
		AddField(i18n.Choose("Memória", "Memory"), memory, true).
		AddField(i18n.Choose("Tempo online", "Uptime"), uptime, true).
		AddField(i18n.Choose("Jogadores", "Players"), dashboardPlayerCount(status), true).
		WithColor(dashboardStatusColor(status)).
		WithFooter(i18n.Choose("Instância AMP: ", "AMP instance: ")+instance.Name, "")

	phase := amp.ApplicationPhaseUnknown
	if status.ApplicationStatus != nil {
		phase = status.ApplicationStatus.Phase()
	}
	actionRow := disgoDiscord.NewActionRow(
		disgoDiscord.NewSuccessButton(i18n.Choose("Iniciar", "Start"), dashboardComponentID("start", instance.Name)).WithDisabled(instance.Running && phase != amp.ApplicationPhaseIdle),
		disgoDiscord.NewDangerButton(i18n.Choose("Parar", "Stop"), dashboardComponentID("stop", instance.Name)).WithDisabled(!instance.Running || phase != amp.ApplicationPhaseOnline),
		disgoDiscord.NewSecondaryButton(i18n.Choose("Reiniciar", "Restart"), dashboardComponentID("restart", instance.Name)).WithDisabled(!instance.Running || phase != amp.ApplicationPhaseOnline),
		disgoDiscord.NewPrimaryButton(i18n.Choose("Atualizar", "Update"), dashboardComponentID("update", instance.Name)),
	)
	components := []disgoDiscord.LayoutComponent{actionRow}
	if event.User().ID == c.ownerUserID && c.ampPublicURL != "" && instance.ID != "" {
		manageURL := strings.TrimRight(c.ampPublicURL, "/") + "/instance/" + url.PathEscape(instance.ID)
		components = append(components, disgoDiscord.NewActionRow(disgoDiscord.NewLinkButton(i18n.Choose("Abrir no AMP", "Open in AMP"), manageURL)))
	}
	message := disgoDiscord.NewMessageUpdate().WithEmbeds(embed).WithComponents(components...)
	_, _ = c.interactions.UpdateInteractionResponse(event.ApplicationID(), event.Token(), message)
	go c.createDetachedCommandAudit(commandAuditRecord{
		UserID: event.User().ID, UserName: event.User().EffectiveName(), ChannelID: event.Channel().ID(),
		Command: i18n.Choose("/painel detalhes", "/dashboard details"), Server: instance.Name, Accepted: true, Reason: i18n.Choose("Consultado pelo painel", "Requested from dashboard"), CreatedAt: time.Now(),
	}, commandAuditPresentation{Phase: commandAuditPhaseCompleted, Result: i18n.Choose("Detalhes exibidos", "Details displayed"), FinalState: i18n.Choose("Sem alteração", "Unchanged"), UpdatedAt: time.Now()})
}

func collectSingleAMPStatus(c *Client, instance amp.ManagedInstance) ampInstanceStatusView {
	statuses := c.collectAMPInstanceStatuses([]amp.ManagedInstance{instance})
	if len(statuses) == 1 {
		return statuses[0]
	}
	return ampInstanceStatusView{Instance: instance}
}

func gameIconURL(game string) string {
	filename := gameIconFilename(game)
	if filename == "" {
		return ""
	}
	return "attachment://" + filename
}

func gameIconFilename(game string) string {
	lower := strings.ToLower(game)
	switch {
	case strings.Contains(lower, "minecraft"):
		return "minecraft.png"
	case strings.Contains(lower, "hytale"):
		return "hytale.png"
	case strings.Contains(lower, "team"):
		return "teamspeak.png"
	case strings.Contains(lower, "palworld"):
		return "palworld.png"
	case strings.Contains(lower, "zomboid"):
		return "project-zomboid.png"
	case strings.Contains(lower, "valheim"):
		return "valheim.png"
	case strings.Contains(lower, "satisfactory"):
		return "satisfactory.png"
	default:
		return ""
	}
}

func gameIconFile(game string) (*disgoDiscord.File, error) {
	filename := gameIconFilename(game)
	if filename == "" {
		return nil, nil
	}
	data, err := gameLogoAssets.ReadFile("game-logos/" + filename)
	if err != nil {
		return nil, fmt.Errorf("não foi possível carregar a logo %s: %w", filename, err)
	}
	return disgoDiscord.NewFile(filename, i18n.Choose("Logo de ", "Logo for ")+game, bytes.NewReader(data)), nil
}

func dashboardGameName(instance amp.ManagedInstance) string {
	game := strings.TrimSpace(instance.Game)
	if game == "" {
		game = strings.TrimSpace(instance.Module)
	}
	if game == "" {
		return i18n.Choose("Desconhecido", "Unknown")
	}
	return game
}
