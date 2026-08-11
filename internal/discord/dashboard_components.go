package discord

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

const (
	dashboardServersPerMessage = 3
	dashboardComponentPrefix   = "ampdash"
)

func buildAMPStatusPages(statuses []ampInstanceStatusView, updatedAt time.Time, defaultAddress string) [][]disgoDiscord.LayoutComponent {
	if len(statuses) == 0 {
		return [][]disgoDiscord.LayoutComponent{{
			disgoDiscord.NewContainer(disgoDiscord.NewTextDisplay("Nenhuma instância controlável foi encontrada.")).WithAccentColor(0x5865F2),
		}}
	}

	pages := make([][]disgoDiscord.LayoutComponent, 0, (len(statuses)+dashboardServersPerMessage-1)/dashboardServersPerMessage)
	for start := 0; start < len(statuses); start += dashboardServersPerMessage {
		end := min(start+dashboardServersPerMessage, len(statuses))
		components := make([]disgoDiscord.LayoutComponent, 0, end-start+1)
		for _, status := range statuses[start:end] {
			components = append(components, buildAMPServerContainer(status, defaultAddress))
		}
		footer := fmt.Sprintf("Atualizado <t:%d:R>", updatedAt.Unix())
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
	game := strings.TrimSpace(status.Instance.Game)
	if game == "" {
		game = strings.TrimSpace(status.Instance.Module)
	}
	if game == "" {
		game = "Desconhecido"
	}
	uptime := "0 min"
	if status.ApplicationStatus != nil {
		uptime = formatAMPUptime(status.ApplicationStatus.Uptime)
	}
	players := dashboardPlayerCount(status)
	cpu, memory := dashboardResourceUsage(status.ApplicationStatus)
	address := dashboardInstanceAddress(instancePresentationOverride{Address: status.Address}, defaultAddress)
	name := ampInstanceDisplayName(status.Instance)

	headerText := disgoDiscord.NewTextDisplay(fmt.Sprintf(
		"## %s\n**Status do servidor**\n%s\n\n**Jogo**\n`%s`",
		name, state, game,
	))
	var header disgoDiscord.ContainerSubComponent = headerText
	if imageURL := gameIconURL(game); imageURL != "" {
		header = disgoDiscord.NewSection(headerText).
			WithAccessory(disgoDiscord.NewThumbnail(imageURL).WithDescription("Capa de " + game + " na Steam"))
	}

	details := disgoDiscord.NewTextDisplay(fmt.Sprintf(
		"**Endereço do servidor**\n`%s`\n\n"+
			"**CPU**　　**Memória**　　**Tempo online**\n`%s`　　`%s`　　`%s`\n\n"+
			"**Jogadores**\n`%s`",
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
		disgoDiscord.NewSuccessButton("Iniciar", dashboardComponentID("start", status.Instance.Name)).WithDisabled(startDisabled),
		disgoDiscord.NewDangerButton("Parar", dashboardComponentID("stop", status.Instance.Name)).WithDisabled(stopDisabled),
		disgoDiscord.NewSecondaryButton("Reiniciar", dashboardComponentID("restart", status.Instance.Name)).WithDisabled(restartDisabled),
		disgoDiscord.NewPrimaryButton("Atualizar", dashboardComponentID("update", status.Instance.Name)),
		disgoDiscord.NewSecondaryButton("Detalhes", dashboardComponentID("details", status.Instance.Name)),
	)

	return disgoDiscord.NewContainer(
		header,
		disgoDiscord.NewSmallSeparator(),
		details,
		disgoDiscord.NewSmallSeparator(),
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
		return "indisponível", "indisponível"
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
	return "indisponível"
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
	return "não configurado"
}

func (c *Client) handleDashboardComponent(event *events.ComponentInteractionCreate) {
	customID := event.Data.CustomID()
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 || parts[0] != dashboardComponentPrefix {
		return
	}
	if event.Channel().ID() != c.notificationChannelID {
		_ = event.CreateMessage(disgoDiscord.NewMessageCreate().WithContent("⛔ Este painel só funciona no canal do AmpControl.").WithEphemeral(true))
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
		ampCommandBypassesPlayerProtection(event.User().ID, event.Member(), c.ownerUserID),
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
		WithAuthor(ampInstanceDisplayName(instance), "", gameIconURL(instance.Game)).
		AddField("Status", icon+" "+state, true).
		AddField("Jogo", instance.Game, true).
		AddField("Endereço", dashboardInstanceAddress(setting, c.gameServerAddress), false).
		AddField("CPU", cpu, true).
		AddField("Memória", memory, true).
		AddField("Tempo online", uptime, true).
		AddField("Jogadores", dashboardPlayerCount(status), true).
		WithColor(dashboardStatusColor(status)).
		WithFooter("Instância AMP: "+instance.Name, "")

	phase := amp.ApplicationPhaseUnknown
	if status.ApplicationStatus != nil {
		phase = status.ApplicationStatus.Phase()
	}
	actionRow := disgoDiscord.NewActionRow(
		disgoDiscord.NewSuccessButton("Iniciar", dashboardComponentID("start", instance.Name)).WithDisabled(instance.Running && phase != amp.ApplicationPhaseIdle),
		disgoDiscord.NewDangerButton("Parar", dashboardComponentID("stop", instance.Name)).WithDisabled(!instance.Running || phase != amp.ApplicationPhaseOnline),
		disgoDiscord.NewSecondaryButton("Reiniciar", dashboardComponentID("restart", instance.Name)).WithDisabled(!instance.Running || phase != amp.ApplicationPhaseOnline),
		disgoDiscord.NewPrimaryButton("Atualizar", dashboardComponentID("update", instance.Name)),
	)
	components := []disgoDiscord.LayoutComponent{actionRow}
	if event.User().ID == c.ownerUserID && c.ampPublicURL != "" && instance.ID != "" {
		manageURL := strings.TrimRight(c.ampPublicURL, "/") + "/instance/" + url.PathEscape(instance.ID)
		components = append(components, disgoDiscord.NewActionRow(disgoDiscord.NewLinkButton("Abrir no AMP", manageURL)))
	}
	message := disgoDiscord.NewMessageUpdate().WithEmbeds(embed).WithComponents(components...)
	_, _ = c.interactions.UpdateInteractionResponse(event.ApplicationID(), event.Token(), message)
	go c.createDetachedCommandAudit(commandAuditRecord{
		UserID: event.User().ID, UserName: event.User().EffectiveName(), ChannelID: event.Channel().ID(),
		Command: "/painel detalhes", Server: instance.Name, Accepted: true, Reason: "Consultado pelo painel", CreatedAt: time.Now(),
	}, commandAuditPresentation{Phase: commandAuditPhaseCompleted, Result: "Detalhes exibidos", FinalState: "Sem alteração", UpdatedAt: time.Now()})
}

func collectSingleAMPStatus(c *Client, instance amp.ManagedInstance) ampInstanceStatusView {
	statuses := c.collectAMPInstanceStatuses([]amp.ManagedInstance{instance})
	if len(statuses) == 1 {
		return statuses[0]
	}
	return ampInstanceStatusView{Instance: instance}
}

func gameIconURL(game string) string {
	lower := strings.ToLower(game)
	appID := ""
	switch {
	case strings.Contains(lower, "palworld"):
		appID = "1623730"
	case strings.Contains(lower, "zomboid"):
		appID = "108600"
	case strings.Contains(lower, "valheim"):
		appID = "892970"
	case strings.Contains(lower, "satisfactory"):
		appID = "526870"
	case strings.Contains(lower, "terraria"):
		appID = "105600"
	case strings.Contains(lower, "7 days"):
		appID = "251570"
	case strings.Contains(lower, "enshrouded"):
		appID = "1203620"
	case strings.Contains(lower, "v rising"):
		appID = "1604030"
	}
	if appID == "" {
		return ""
	}
	return "https://cdn.cloudflare.steamstatic.com/steam/apps/" + appID + "/header.jpg"
}
