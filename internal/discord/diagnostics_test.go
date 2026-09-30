package discord

import (
	"strings"
	"testing"
	"time"
)

func TestFormatDiagnosticsReportUsesWorstSeverity(t *testing.T) {
	report := formatDiagnosticsReport(
		[]diagnosticItem{
			{Name: "Discord", Detail: "OK", Severity: diagnosticHealthy},
			{Name: "RCON", Detail: "Credencial ausente", Severity: diagnosticWarning},
			{Name: "AMP", Detail: "Indisponível", Severity: diagnosticFailure},
		},
		time.Unix(1_700_000_000, 0),
	)

	for _, expected := range []string{
		"🔴 Falha detectada",
		"🟢 Discord",
		"🟡 RCON",
		"🔴 AMP",
		"<t:1700000000:R>",
	} {
		if !strings.Contains(report, expected) {
			t.Fatalf("relatório não contém %q:\n%s", expected, report)
		}
	}
}

func TestMaxDiagnosticSeverity(t *testing.T) {
	if actual := maxDiagnosticSeverity(
		diagnosticWarning,
		diagnosticHealthy,
	); actual != diagnosticWarning {
		t.Fatalf("severidade regrediu para %d", actual)
	}
	if actual := maxDiagnosticSeverity(
		diagnosticWarning,
		diagnosticFailure,
	); actual != diagnosticFailure {
		t.Fatalf("falha não prevaleceu: %d", actual)
	}
}

func TestMissingInstancesDiagnostic(t *testing.T) {
	if _, ok := missingInstancesDiagnostic(nil); ok {
		t.Fatal("sem instâncias ausentes o diagnóstico não deveria ganhar item")
	}
	item, ok := missingInstancesDiagnostic([]string{"AIO01", "Velha02"})
	if !ok || item.Severity != diagnosticWarning {
		t.Fatalf("instâncias ausentes deveriam gerar um aviso: %+v", item)
	}
	if !strings.Contains(item.Detail, "AIO01, Velha02") || !strings.Contains(item.Detail, "config/idle.json") {
		t.Fatalf("o aviso deveria listar as instâncias e dizer onde limpar: %q", item.Detail)
	}
}
