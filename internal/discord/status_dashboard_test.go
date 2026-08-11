package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
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

func TestBuildAMPStatusPagesUsesThreeVerticalCardsPerMessage(t *testing.T) {
	statuses := make([]ampInstanceStatusView, 6)
	for index := range statuses {
		statuses[index].Instance = amp.ManagedInstance{
			Name: fmt.Sprintf("Server%02d", index), FriendlyName: fmt.Sprintf("Servidor %d", index+1), Game: "Minecraft",
		}
	}
	pages := buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22")
	if len(pages) != 2 {
		t.Fatalf("seis servidores deveriam ocupar duas mensagens: %#v", pages)
	}
	for pageIndex, page := range pages {
		if len(page) != 4 {
			t.Fatalf("página %d deveria ter três cartões e um rodapé: %d", pageIndex, len(page))
		}
		for index := 0; index < 3; index++ {
			if _, ok := page[index].(disgoDiscord.ContainerComponent); !ok {
				t.Fatalf("componente %d da página %d deveria ser um cartão", index, pageIndex)
			}
		}
	}
}

func TestBuildAMPStatusPagesDoesNotEmitNullSectionAccessory(t *testing.T) {
	statuses := []ampInstanceStatusView{
		{Instance: amp.ManagedInstance{Name: "Minecraft01", Game: "Minecraft"}},
		{Instance: amp.ManagedInstance{Name: "Valheim01", Game: "Valheim"}},
	}
	pages := buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22")
	payload, err := json.Marshal(pages)
	if err != nil {
		t.Fatalf("não foi possível serializar o painel: %v", err)
	}
	if bytes.Contains(payload, []byte(`"accessory":null`)) {
		t.Fatalf("painel contém accessory nulo rejeitado pelo Discord: %s", payload)
	}
}

func TestGameIconURLUsesSteamLogosAndOfficialNonSteamAssets(t *testing.T) {
	tests := map[string]string{
		"PalWorld (Modded)": "/1623730/logo.png",
		"Project Zomboid":   "/108600/logo.png",
		"Valheim":           "/892970/logo.png",
		"Satisfactory":      "/526870/logo.png",
		"Minecraft":         "minecraft.net/",
		"Hytale":            "accounts.hytale.com/",
		"TeamSpeak":         "teamspeak.com/",
	}
	for game, expected := range tests {
		if actual := gameIconURL(game); !strings.Contains(actual, expected) {
			t.Fatalf("imagem inesperada para %s: %q", game, actual)
		}
	}
}

func TestStatusDashboardStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord_status.json")
	expected := statusDashboardState{
		MessageID:           "123456789",
		GuideMessageID:      "987654321",
		MessageOrderVersion: statusDashboardMessageOrderVersion,
	}

	if err := saveStatusDashboardState(path, expected); err != nil {
		t.Fatalf("saveStatusDashboardState retornou erro: %v", err)
	}

	actual, err := loadStatusDashboardState(path)
	if err != nil {
		t.Fatalf("loadStatusDashboardState retornou erro: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
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
	guideID := snowflake.ID(150)

	if shouldDeleteChannelMessage(
		dashboardID,
		dashboardID,
		guideID,
		cutoff.Add(-time.Hour),
		cutoff,
	) {
		t.Fatal("o painel fixo nunca deve ser apagado")
	}

	if shouldDeleteChannelMessage(
		guideID,
		dashboardID,
		guideID,
		cutoff.Add(-time.Hour),
		cutoff,
	) {
		t.Fatal("o guia fixo nunca deve ser apagado")
	}

	if !shouldDeleteChannelMessage(
		snowflake.ID(200),
		dashboardID,
		guideID,
		cutoff.Add(-time.Hour),
		cutoff,
	) {
		t.Fatal("qualquer outra mensagem expirada deve ser apagada")
	}

	if shouldDeleteChannelMessage(
		snowflake.ID(300),
		dashboardID,
		guideID,
		cutoff.Add(time.Minute),
		cutoff,
	) {
		t.Fatal("mensagem ainda dentro do TTL não deve ser apagada")
	}
}

func TestStatusDashboardOrderMigrationRunsOnlyOnce(t *testing.T) {
	legacyState := statusDashboardState{
		MessageID:      "100",
		GuideMessageID: "200",
	}
	if !statusDashboardNeedsOrderMigration(legacyState) {
		t.Fatal("o estado legado deveria solicitar a migração de ordem")
	}

	migratedState := legacyState
	migratedState.MessageOrderVersion = statusDashboardMessageOrderVersion
	if statusDashboardNeedsOrderMigration(migratedState) {
		t.Fatal("a migração concluída não deveria ser repetida")
	}

	withoutGuide := statusDashboardState{MessageID: "100"}
	if statusDashboardNeedsOrderMigration(withoutGuide) {
		t.Fatal("não há ordem para migrar enquanto o guia ainda não existe")
	}
}

func TestBuildAMPCommandGuideEmbedsDocumentsEveryCommand(t *testing.T) {
	embeds := buildAMPCommandGuideEmbeds()
	if len(embeds) != 7 {
		t.Fatalf("quantidade inesperada de embeds do guia: %d", len(embeds))
	}

	expectedCommands := []string{
		"/amp status",
		"/amp iniciar",
		"/amp parar",
		"/amp reiniciar",
		"/amp desligar",
		"/amp atualizar",
	}

	for _, command := range expectedCommands {
		found := false
		for _, embed := range embeds {
			if strings.Contains(embed.Title, command) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("o guia não documenta %s", command)
		}
	}

	if !strings.Contains(embeds[0].Description, "não funcionam em outros canais") {
		t.Fatal("o guia deveria explicar a restrição de canal")
	}
	guideText := ""
	for _, embed := range embeds {
		guideText += "\n" + embed.Title + "\n" + embed.Description
	}
	if !strings.Contains(guideText, "Proteção de partidas") ||
		!strings.Contains(guideText, "jogadores conectados") {
		t.Fatal("o guia deveria explicar a proteção de partidas em andamento")
	}
	for _, embed := range embeds {
		if strings.Contains(embed.Title, "/ampconfig") ||
			strings.Contains(embed.Description, "/ampconfig") {
			t.Fatal("o guia público não deve revelar comandos privados de configuração")
		}
	}
}

func TestAMPCommandGuideFitsDiscordLimits(t *testing.T) {
	embeds := buildAMPCommandGuideEmbeds()
	if len(commandGuideContent) > 2000 {
		t.Fatal("o conteúdo do guia excede o limite do Discord")
	}
	if len(embeds) > 10 {
		t.Fatal("o guia excede o limite de embeds do Discord")
	}

	totalCharacters := 0
	fieldCount := 0
	for _, embed := range embeds {
		totalCharacters += len(embed.Title) + len(embed.Description)
		fieldCount += len(embed.Fields)
		for _, field := range embed.Fields {
			if len(field.Name) > 256 || len(field.Value) > 1024 {
				t.Fatalf("campo do guia excede os limites: %q", field.Name)
			}
			totalCharacters += len(field.Name) + len(field.Value)
		}
		if embed.Footer != nil {
			totalCharacters += len(embed.Footer.Text)
		}
	}

	if fieldCount > 25 {
		t.Fatalf("o guia excede o limite de campos: %d", fieldCount)
	}
	if totalCharacters > 6000 {
		t.Fatalf("o guia excede o limite total de caracteres: %d", totalCharacters)
	}
}

func TestBuildAMPStatusEmbedsUsesConfiguredMaximumWhenCountUnavailable(t *testing.T) {
	embeds := buildAMPStatusEmbeds([]ampInstanceStatusView{{
		Instance: amp.ManagedInstance{
			Name: "Servidor01", FriendlyName: "Servidor", Game: "Minecraft", Running: true,
		},
		PlayerMaxOverride: 40,
	}}, time.Now())
	if len(embeds) < 1 || len(embeds[0].Fields) < 1 {
		t.Fatal("painel não contém o servidor")
	}
	if !strings.Contains(embeds[0].Fields[0].Value, "?/40") {
		t.Fatalf("limite personalizado não apareceu: %q", embeds[0].Fields[0].Value)
	}
}
