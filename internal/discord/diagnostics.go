package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/disgoorg/disgo/events"
)

const diagnosticsTimeout = 25 * time.Second

type diagnosticSeverity int

const (
	diagnosticHealthy diagnosticSeverity = iota
	diagnosticWarning
	diagnosticFailure
)

type diagnosticItem struct {
	Name     string
	Detail   string
	Severity diagnosticSeverity
}

type dashboardDiagnosticsSnapshot struct {
	DiscordConnectedAt   time.Time
	LastDashboardSuccess time.Time
	LastDashboardFailure time.Time
	LastDashboardError   string
}

func (c *Client) handleAMPDiagnosticsCommand(
	event *events.ApplicationCommandInteractionCreate,
) {
	if !c.deferAMPInteraction(event) {
		return
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		diagnosticsTimeout,
	)
	defer cancel()

	items := c.collectDiagnostics(ctx)
	c.updateInteractionMessage(
		event,
		formatDiagnosticsReport(items, time.Now()),
	)
	c.deleteInteractionResponseLater(event, 60*time.Second)
}

func (c *Client) collectDiagnostics(
	ctx context.Context,
) []diagnosticItem {
	items := make([]diagnosticItem, 0, 5)
	snapshot := c.dashboardDiagnosticsSnapshot()

	discordDetail := i18n.Choose("Gateway conectado", "Gateway connected")
	if !snapshot.DiscordConnectedAt.IsZero() {
		discordDetail += " " + formatDiagnosticRelative(snapshot.DiscordConnectedAt)
	}
	if err := c.validateCommandAuditChannel(); err != nil {
		items = append(items, diagnosticItem{
			Name:     "Discord",
			Detail:   discordDetail + i18n.Choose("; canal de auditoria indisponível: ", "; audit channel unavailable: ") + i18n.Text(sanitizeAuditText(err.Error(), 260)),
			Severity: diagnosticFailure,
		})
	} else {
		items = append(items, diagnosticItem{
			Name: "Discord",
			Detail: fmt.Sprintf(
				i18n.Choose("%s; painel <#%s>; auditoria <#%s>", "%s; dashboard <#%s>; audit <#%s>"),
				discordDetail,
				c.notificationChannelID,
				c.auditChannelID,
			),
			Severity: diagnosticHealthy,
		})
	}

	instances, ampErr := c.ampClient.DiscoverManagedInstances(
		ctx,
		c.adsURL,
	)
	if ampErr != nil {
		items = append(items, diagnosticItem{
			Name:     i18n.Choose("API ADS do AMP", "AMP ADS API"),
			Detail:   i18n.Text(sanitizeAuditText(ampErr.Error(), 320)),
			Severity: diagnosticFailure,
		})
	} else {
		running := 0
		for _, instance := range instances {
			if instance.Running {
				running++
			}
		}
		items = append(items, diagnosticItem{
			Name: i18n.Choose("API ADS do AMP", "AMP ADS API"),
			Detail: fmt.Sprintf(
				i18n.Choose("Autenticação e inventário válidos; %d instâncias encontradas, %d ligadas", "Authentication and inventory valid; %d instances found, %d running"),
				len(instances),
				running,
			),
			Severity: diagnosticHealthy,
		})
	}

	idleSnapshot, idleAvailable := c.idleDiagnosticsSnapshot()
	if !idleAvailable {
		items = append(items,
			diagnosticItem{
				Name:     i18n.Choose("Motor de Idle", "Idle engine"),
				Detail:   i18n.Choose("Provedor de diagnóstico não está disponível", "Diagnostics provider is unavailable"),
				Severity: diagnosticFailure,
			},
			diagnosticItem{
				Name:     "RCON",
				Detail:   i18n.Choose("Não foi possível consultar os detectores configurados", "Could not query configured detectors"),
				Severity: diagnosticWarning,
			},
		)
	} else {
		idleSeverity := diagnosticHealthy
		idleStatus := i18n.Choose("Ativo", "Active")
		if !idleSnapshot.Running {
			idleSeverity = diagnosticFailure
			idleStatus = i18n.Choose("Parado", "Stopped")
		}

		idleDetail := fmt.Sprintf(
			i18n.Choose("%s; %d cadastrados, %d ativos, %d em observação; ciclo de %s", "%s; %d registered, %d active, %d in observe mode; %s cycle"),
			idleStatus,
			idleSnapshot.RegisteredServers,
			idleSnapshot.ActiveServers,
			idleSnapshot.ObserveServers,
			formatCommandAuditDuration(idleSnapshot.CheckInterval),
		)
		if !idleSnapshot.LastEventAt.IsZero() {
			idleDetail += i18n.Choose("; último ciclo ", "; last cycle ") + formatDiagnosticRelative(idleSnapshot.LastEventAt)
		}
		if !idleSnapshot.LastErrorAt.IsZero() &&
			time.Since(idleSnapshot.LastErrorAt) <= 5*time.Minute {
			idleSeverity = maxDiagnosticSeverity(idleSeverity, diagnosticWarning)
			idleDetail += i18n.Choose("; falha recente: ", "; recent failure: ") + i18n.Text(sanitizeAuditText(idleSnapshot.LastError, 220))
		}
		items = append(items, diagnosticItem{
			Name:     i18n.Choose("Motor de Idle", "Idle engine"),
			Detail:   idleDetail,
			Severity: idleSeverity,
		})

		rconSeverity := diagnosticHealthy
		rconDetail := i18n.Choose("Nenhum detector RCON configurado", "No RCON detector configured")
		if idleSnapshot.RCONServers > 0 {
			rconDetail = fmt.Sprintf(
				i18n.Choose("%d detectores configurados; %d com endereço e credencial carregados", "%d detectors configured; %d with address and credential loaded"),
				idleSnapshot.RCONServers,
				idleSnapshot.RCONReadyServers,
			)
			if idleSnapshot.RCONReadyServers != idleSnapshot.RCONServers {
				rconSeverity = diagnosticWarning
			}
		}
		items = append(items, diagnosticItem{
			Name:     "RCON",
			Detail:   rconDetail,
			Severity: rconSeverity,
		})
	}

	dashboardSeverity := diagnosticHealthy
	dashboardDetail := i18n.Choose("Nenhuma atualização bem-sucedida foi registrada nesta execução", "No successful update was recorded in this run")
	if snapshot.LastDashboardSuccess.IsZero() {
		dashboardSeverity = diagnosticWarning
	} else {
		dashboardDetail = i18n.Choose("Última atualização bem-sucedida ", "Last successful update ") +
			formatDiagnosticRelative(snapshot.LastDashboardSuccess)
	}
	if strings.TrimSpace(snapshot.LastDashboardError) != "" {
		dashboardSeverity = diagnosticFailure
		dashboardDetail += i18n.Choose("; erro: ", "; error: ") + i18n.Text(sanitizeAuditText(snapshot.LastDashboardError, 260))
	}
	items = append(items, diagnosticItem{
		Name:     i18n.Choose("Painel fixo", "Pinned dashboard"),
		Detail:   dashboardDetail,
		Severity: dashboardSeverity,
	})

	return items
}

func (c *Client) dashboardDiagnosticsSnapshot() dashboardDiagnosticsSnapshot {
	c.diagnosticsMu.RLock()
	defer c.diagnosticsMu.RUnlock()
	return dashboardDiagnosticsSnapshot{
		DiscordConnectedAt:   c.discordConnectedAt,
		LastDashboardSuccess: c.lastDashboardSuccess,
		LastDashboardFailure: c.lastDashboardFailure,
		LastDashboardError:   c.lastDashboardError,
	}
}

func (c *Client) idleDiagnosticsSnapshot() (
	IdleDiagnosticsSnapshot,
	bool,
) {
	c.idleRegistrationMu.RLock()
	provider := c.idleDiagnosticsProvider
	c.idleRegistrationMu.RUnlock()
	if provider == nil {
		return IdleDiagnosticsSnapshot{}, false
	}
	return provider.IdleDiagnostics(), true
}

func formatDiagnosticsReport(
	items []diagnosticItem,
	checkedAt time.Time,
) string {
	overall := diagnosticHealthy
	for _, item := range items {
		overall = maxDiagnosticSeverity(overall, item.Severity)
	}

	var builder strings.Builder
	builder.WriteString(i18n.Choose("## 🩺 Diagnóstico do AmpControl\n", "## 🩺 AmpControl diagnostics\n"))
	builder.WriteString(i18n.Choose("**Resultado geral:** ", "**Overall result:** "))
	builder.WriteString(diagnosticSeverityLabel(overall))
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf(i18n.Choose("Verificado <t:%d:R>\n", "Checked <t:%d:R>\n"), checkedAt.Unix()))

	for _, item := range items {
		builder.WriteString("\n**")
		builder.WriteString(diagnosticSeverityIcon(item.Severity))
		builder.WriteString(" ")
		builder.WriteString(item.Name)
		builder.WriteString("**\n")
		builder.WriteString(item.Detail)
		builder.WriteString("\n")
	}

	return builder.String()
}

func diagnosticSeverityIcon(severity diagnosticSeverity) string {
	switch severity {
	case diagnosticFailure:
		return "🔴"
	case diagnosticWarning:
		return "🟡"
	default:
		return "🟢"
	}
}

func diagnosticSeverityLabel(severity diagnosticSeverity) string {
	switch severity {
	case diagnosticFailure:
		return i18n.Choose("🔴 Falha detectada", "🔴 Failure detected")
	case diagnosticWarning:
		return i18n.Choose("🟡 Atenção necessária", "🟡 Attention required")
	default:
		return i18n.Choose("🟢 Saudável", "🟢 Healthy")
	}
}

func maxDiagnosticSeverity(
	left diagnosticSeverity,
	right diagnosticSeverity,
) diagnosticSeverity {
	if right > left {
		return right
	}
	return left
}

func formatDiagnosticRelative(value time.Time) string {
	if value.IsZero() {
		return i18n.Choose("nunca", "never")
	}
	return fmt.Sprintf("<t:%d:R>", value.Unix())
}
