package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

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

	discordDetail := "Gateway conectado"
	if !snapshot.DiscordConnectedAt.IsZero() {
		discordDetail += " " + formatDiagnosticRelative(snapshot.DiscordConnectedAt)
	}
	if err := c.validateCommandAuditChannel(); err != nil {
		items = append(items, diagnosticItem{
			Name:     "Discord",
			Detail:   discordDetail + "; canal de auditoria indisponível: " + sanitizeAuditText(err.Error(), 260),
			Severity: diagnosticFailure,
		})
	} else {
		items = append(items, diagnosticItem{
			Name: "Discord",
			Detail: fmt.Sprintf(
				"%s; painel <#%s>; auditoria <#%s>",
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
			Name:     "API ADS do AMP",
			Detail:   sanitizeAuditText(ampErr.Error(), 320),
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
			Name: "API ADS do AMP",
			Detail: fmt.Sprintf(
				"Autenticação e inventário válidos; %d instâncias encontradas, %d ligadas",
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
				Name:     "Motor de Idle",
				Detail:   "Provedor de diagnóstico não está disponível",
				Severity: diagnosticFailure,
			},
			diagnosticItem{
				Name:     "RCON",
				Detail:   "Não foi possível consultar os detectores configurados",
				Severity: diagnosticWarning,
			},
		)
	} else {
		idleSeverity := diagnosticHealthy
		idleStatus := "Ativo"
		if !idleSnapshot.Running {
			idleSeverity = diagnosticFailure
			idleStatus = "Parado"
		}

		idleDetail := fmt.Sprintf(
			"%s; %d cadastrados, %d ativos, %d em observação; ciclo de %s",
			idleStatus,
			idleSnapshot.RegisteredServers,
			idleSnapshot.ActiveServers,
			idleSnapshot.ObserveServers,
			formatCommandAuditDuration(idleSnapshot.CheckInterval),
		)
		if !idleSnapshot.LastEventAt.IsZero() {
			idleDetail += "; último ciclo " + formatDiagnosticRelative(idleSnapshot.LastEventAt)
		}
		if !idleSnapshot.LastErrorAt.IsZero() &&
			time.Since(idleSnapshot.LastErrorAt) <= 5*time.Minute {
			idleSeverity = maxDiagnosticSeverity(idleSeverity, diagnosticWarning)
			idleDetail += "; falha recente: " + sanitizeAuditText(idleSnapshot.LastError, 220)
		}
		items = append(items, diagnosticItem{
			Name:     "Motor de Idle",
			Detail:   idleDetail,
			Severity: idleSeverity,
		})

		rconSeverity := diagnosticHealthy
		rconDetail := "Nenhum detector RCON configurado"
		if idleSnapshot.RCONServers > 0 {
			rconDetail = fmt.Sprintf(
				"%d detectores configurados; %d com endereço e credencial carregados",
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
	dashboardDetail := "Nenhuma atualização bem-sucedida foi registrada nesta execução"
	if snapshot.LastDashboardSuccess.IsZero() {
		dashboardSeverity = diagnosticWarning
	} else {
		dashboardDetail = "Última atualização bem-sucedida " +
			formatDiagnosticRelative(snapshot.LastDashboardSuccess)
	}
	if strings.TrimSpace(snapshot.LastDashboardError) != "" {
		dashboardSeverity = diagnosticFailure
		dashboardDetail += "; erro: " + sanitizeAuditText(snapshot.LastDashboardError, 260)
	}
	items = append(items, diagnosticItem{
		Name:     "Painel fixo",
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
	builder.WriteString("## 🩺 Diagnóstico do AmpControl\n")
	builder.WriteString("**Resultado geral:** ")
	builder.WriteString(diagnosticSeverityLabel(overall))
	builder.WriteString("\n")
	builder.WriteString(fmt.Sprintf("Verificado <t:%d:R>\n", checkedAt.Unix()))

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
		return "🔴 Falha detectada"
	case diagnosticWarning:
		return "🟡 Atenção necessária"
	default:
		return "🟢 Saudável"
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
		return "nunca"
	}
	return fmt.Sprintf("<t:%d:R>", value.Unix())
}
