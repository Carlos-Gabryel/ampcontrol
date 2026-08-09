package discord

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/amp"
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

	message := buildAMPStatusMessage([]ampInstanceStatusView{
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
	})

	expectedParts := []string{
		"🟡 **Alamama** — **Idle**",
		"Jogo: `Palworld`",
		"Jogadores: `0/32`",
		"🟢 **HyLabama** — **Online**",
		"Tempo online: `3h 24min`",
		"Jogadores: `2/100`",
		"🔴 **Vanilla - 2025** — **Offline**",
		"Jogo: `Minecraft`",
	}

	for _, part := range expectedParts {
		if !strings.Contains(message, part) {
			t.Fatalf("mensagem não contém %q:\n%s", part, message)
		}
	}

	if strings.Contains(message, "💤") || strings.Contains(message, "ZZZ") {
		t.Fatalf("mensagem ainda contém o ícone antigo de Idle:\n%s", message)
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

func TestStatusDashboardMessageFitsDiscordLimit(t *testing.T) {
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

	message := buildAMPStatusMessage(statuses)
	if len(message) > 2000 {
		t.Fatalf("painel excede o limite do Discord: %d bytes", len(message))
	}
}
