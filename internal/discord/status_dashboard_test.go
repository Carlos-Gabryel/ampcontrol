package discord

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/disgoorg/snowflake/v2"
)

func TestBuildAMPStatusMessageUsesRequestedPresentation(t *testing.T) {
	idleStatus := amp.ApplicationStatus{
		State:  amp.ApplicationStateStopped,
		Uptime: "0:00:00:00",
	}
	onlineStatus := amp.ApplicationStatus{
		State:  amp.ApplicationStateReady,
		Uptime: "0:03:24:12",
	}

	embed := buildAMPStatusEmbed([]ampInstanceStatusView{
		{
			Instance: amp.ManagedInstance{
				Name:         "AlamamaPal01",
				FriendlyName: "Alamama",
				Game:         "Palworld",
				Running:      true,
			},
			ApplicationStatus: &idleStatus,
			PlayerCounts: &amp.PlayerCounts{
				Current: 0,
				Maximum: 32,
			},
		},
		{
			Instance: amp.ManagedInstance{
				Name:         "HyLabama01",
				FriendlyName: "HyLabama",
				Game:         "Hytale",
				Running:      true,
			},
			ApplicationStatus: &onlineStatus,
			PlayerCounts: &amp.PlayerCounts{
				Current: 2,
				Maximum: 100,
			},
		},
		{
			Instance: amp.ManagedInstance{
				Name:         "Vanilla-202501",
				FriendlyName: "Vanilla - 2025",
				Game:         "Minecraft",
				Running:      false,
			},
		},
	}, time.Unix(1_700_000_000, 0))

	if embed.Title != "— Estado dos servidores" {
		t.Fatalf("título inesperado: %q", embed.Title)
	}
	if len(embed.Fields) != 3 {
		t.Fatalf("quantidade inesperada de campos: %d", len(embed.Fields))
	}

	expectedNames := []string{
		"🟡 Alamama — Idle",
		"🟢 HyLabama — Online",
		"🔴 Vanilla - 2025 — Offline",
	}
	expectedValues := []string{
		"**Jogo:** `Palworld`\n**Tempo online:** `0 min`\n**Jogadores:** `0/32`",
		"**Jogo:** `Hytale`\n**Tempo online:** `3h 24min`\n**Jogadores:** `2/100`",
		"**Jogo:** `Minecraft`\n**Tempo online:** `0 min`\n**Jogadores:** `0/?`",
	}

	for index, field := range embed.Fields {
		if field.Name != expectedNames[index] {
			t.Fatalf("nome do campo %d inesperado: %q", index, field.Name)
		}
		if field.Value != expectedValues[index] {
			t.Fatalf("valor do campo %d inesperado: %q", index, field.Value)
		}
		if field.Inline == nil || !*field.Inline {
			t.Fatalf("campo %d deveria usar o grid inline", index)
		}
	}
}

func TestStatusDashboardStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord_status.json")
	expected := statusDashboardState{MessageID: "123456789"}

	if err := saveStatusDashboardState(path, expected); err != nil {
		t.Fatalf("saveStatusDashboardState retornou erro: %v", err)
	}

	actual, err := loadStatusDashboardState(path)
	if err != nil {
		t.Fatalf("loadStatusDashboardState retornou erro: %v", err)
	}

	if actual != expected {
		t.Fatalf("estado inesperado: %+v", actual)
	}
}

func TestFormatAMPUptime(t *testing.T) {
	tests := map[string]string{
		"0:00:00:00": "0 min",
		"0:00:00:35": "<1 min",
		"0:00:15:00": "15 min",
		"0:03:24:12": "3h 24min",
		"2:05:00:00": "2d 5h",
		"inválido":   "indisponível",
	}

	for input, expected := range tests {
		if actual := formatAMPUptime(input); actual != expected {
			t.Fatalf(
				"formatAMPUptime(%q)=%q; esperado=%q",
				input,
				actual,
				expected,
			)
		}
	}
}

func TestStatusDashboardEmbedFitsDiscordLimits(t *testing.T) {
	statuses := make([]ampInstanceStatusView, 11)
	for index := range statuses {
		status := amp.ApplicationStatus{
			State:  amp.ApplicationStateReady,
			Uptime: "12:23:59:59",
		}
		statuses[index] = ampInstanceStatusView{
			Instance: amp.ManagedInstance{
				Name:         "ServidorMuitoLongo",
				FriendlyName: "Servidor com um nome consideravelmente longo",
				Game:         "Jogo com um nome consideravelmente longo",
				Running:      true,
			},
			ApplicationStatus: &status,
			PlayerCounts: &amp.PlayerCounts{
				Current: 100,
				Maximum: 100,
			},
		}
	}

	embed := buildAMPStatusEmbed(statuses, time.Now())
	if len(embed.Fields) > 25 {
		t.Fatalf("painel excede o limite de campos: %d", len(embed.Fields))
	}
	for index, field := range embed.Fields {
		if len(field.Name) > 256 {
			t.Fatalf("nome do campo %d excede o limite", index)
		}
		if len(field.Value) > 1024 {
			t.Fatalf("valor do campo %d excede o limite", index)
		}
	}
}

func TestShouldDeleteEveryExpiredMessageExceptDashboard(t *testing.T) {
	cutoff := time.Now()
	dashboardID := snowflake.ID(100)

	if shouldDeleteChannelMessage(
		dashboardID,
		dashboardID,
		cutoff.Add(-time.Hour),
		cutoff,
	) {
		t.Fatal("a mensagem fixa nunca deve ser apagada")
	}

	if !shouldDeleteChannelMessage(
		snowflake.ID(200),
		dashboardID,
		cutoff.Add(-time.Hour),
		cutoff,
	) {
		t.Fatal("qualquer outra mensagem expirada deve ser apagada")
	}

	if shouldDeleteChannelMessage(
		snowflake.ID(300),
		dashboardID,
		cutoff.Add(time.Minute),
		cutoff,
	) {
		t.Fatal("mensagem ainda dentro do TTL não deve ser apagada")
	}
}
