package discord

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/disgoorg/snowflake/v2"
)

func TestBuildAMPStatusEmbedsUsesReadableTwoColumnGrid(t *testing.T) {
	idleStatus := amp.ApplicationStatus{
		State:  amp.ApplicationStateStopped,
		Uptime: "0:00:00:00",
	}
	onlineStatus := amp.ApplicationStatus{
		State:  amp.ApplicationStateReady,
		Uptime: "0:03:24:12",
	}

	embeds := buildAMPStatusEmbeds([]ampInstanceStatusView{
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

	if len(embeds) != 3 {
		t.Fatalf("quantidade inesperada de embeds: %d", len(embeds))
	}
	if embeds[0].Title != "" {
		t.Fatalf("o título deveria ficar fora do embed: %q", embeds[0].Title)
	}

	expectedNames := []string{
		"🟡 Alamama",
		"🟢 HyLabama",
		"🔴 Vanilla - 2025",
	}
	expectedValues := []string{
		"**Jogo:** `Palworld`\n**Tempo online:** `0 min`\n**Jogadores:** `0/32`\n──────────────",
		"**Jogo:** `Hytale`\n**Tempo online:** `3h 24min`\n**Jogadores:** `2/100`\n──────────────",
		"**Jogo:** `Minecraft`\n**Tempo online:** `0 min`\n**Jogadores:** `0/?`\n──────────────",
	}

	serverFields := []struct {
		embed int
		field int
	}{
		{embed: 0, field: 0},
		{embed: 0, field: 1},
		{embed: 1, field: 0},
	}
	for index, location := range serverFields {
		field := embeds[location.embed].Fields[location.field]
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

	if len(embeds[1].Fields) != 2 || embeds[1].Fields[1].Value != "\u200b" {
		t.Fatal("a última linha deveria manter a segunda coluna vazia")
	}
	if embeds[2].Description !=
		"**Legenda:**  🟢 Online   •   🟡 Idle   •   🔴 Offline" {
		t.Fatalf("legenda inesperada: %q", embeds[2].Description)
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

	embeds := buildAMPStatusEmbeds(statuses, time.Now())
	if len(embeds) > 10 {
		t.Fatalf("painel excede o limite de embeds: %d", len(embeds))
	}
	fieldCount := 0
	for embedIndex, embed := range embeds {
		fieldCount += len(embed.Fields)
		for fieldIndex, field := range embed.Fields {
			if len(field.Name) > 256 {
				t.Fatalf(
					"nome do campo %d/%d excede o limite",
					embedIndex,
					fieldIndex,
				)
			}
			if len(field.Value) > 1024 {
				t.Fatalf(
					"valor do campo %d/%d excede o limite",
					embedIndex,
					fieldIndex,
				)
			}
		}
	}
	if fieldCount > 25 {
		t.Fatalf("painel excede o limite total de campos: %d", fieldCount)
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
