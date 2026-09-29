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

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestBuildAMPStatusPagesUsesOneCardPerMessage(t *testing.T) {
	statuses := make([]ampInstanceStatusView, 6)
	for index := range statuses {
		statuses[index].Instance = amp.ManagedInstance{
			Name: fmt.Sprintf("Server%02d", index), FriendlyName: fmt.Sprintf("Servidor %d", index+1), Game: "Minecraft",
		}
	}
	pages := buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22")
	if len(pages) != 6 {
		t.Fatalf("seis servidores deveriam ocupar seis mensagens: %#v", pages)
	}
	for pageIndex, page := range pages {
		if len(page) != 2 {
			t.Fatalf("página %d deveria ter um cartão e um rodapé: %d", pageIndex, len(page))
		}
		if _, ok := page[0].(disgoDiscord.ContainerComponent); !ok {
			t.Fatalf("primeiro componente da página %d deveria ser um cartão", pageIndex)
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

func TestGameIconURLUsesNormalizedRepositoryAssets(t *testing.T) {
	tests := map[string]string{
		"PalWorld (Modded)": "palworld.png",
		"Project Zomboid":   "project-zomboid.png",
		"Valheim":           "valheim.png",
		"Satisfactory":      "satisfactory.png",
		"Minecraft":         "minecraft.png",
		"Hytale":            "hytale.png",
		"TeamSpeak":         "teamspeak.png",
	}
	for game, expected := range tests {
		actual := gameIconURL(game)
		if actual != "attachment://"+expected {
			t.Fatalf("imagem inesperada para %s: %q", game, actual)
		}
		file, err := gameIconFile(game)
		if err != nil || file == nil || file.Name != expected {
			t.Fatalf("asset embutido inesperado para %s: file=%v err=%v", game, file, err)
		}
	}
	if actual := gameIconURL("Jogo desconhecido"); actual != "" {
		t.Fatalf("jogo sem asset deveria omitir a imagem: %q", actual)
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

func TestBuildAMPCommandGuideEmbedsDocumentsEveryCommand(t *testing.T) {
	embeds := buildAMPCommandGuideEmbeds(true)
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
	embeds := buildAMPCommandGuideEmbeds(true)
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
