package discord

import (
	"encoding/json"
	"testing"
	"time"

	disgoDiscord "github.com/disgoorg/disgo/discord"
)

func TestFormatCommandAuditPath(t *testing.T) {
	tests := map[string]string{
		"/amp/iniciar":              "/amp iniciar",
		"/ampconfig/idle-adicionar": "/ampconfig idle-adicionar",
		"amp/status":                "/amp status",
		"":                          "/",
	}

	for input, expected := range tests {
		if actual := formatCommandAuditPath(input); actual != expected {
			t.Fatalf("formatação de %q: obtido %q, esperado %q", input, actual, expected)
		}
	}
}

func TestClassifyCommandAuditResponse(t *testing.T) {
	tests := []struct {
		content  string
		phase    commandAuditPhase
		terminal bool
	}{
		{"⏳ Iniciando **Valheim**", commandAuditPhaseRunning, false},
		{"⏳ Starting **Valheim**", commandAuditPhaseRunning, false},
		{"✅ O jogo foi iniciado.", commandAuditPhaseCompleted, true},
		{"❌ Não foi possível iniciar.", commandAuditPhaseFailed, true},
		{"⛔ Canal não autorizado.", commandAuditPhaseRefused, true},
		{"⚠️ Confirmação ausente.", commandAuditPhaseFailed, true},
		{"⏳ O servidor já possui uma operação em andamento.", commandAuditPhaseFailed, true},
		{"⏳ The server already has an operation in progress.", commandAuditPhaseFailed, true},
	}

	for _, test := range tests {
		phase, terminal := classifyCommandAuditResponse(test.content)
		if phase != test.phase || terminal != test.terminal {
			t.Fatalf(
				"classificação de %q: obtido (%s, %t), esperado (%s, %t)",
				test.content,
				phase,
				terminal,
				test.phase,
				test.terminal,
			)
		}
	}
}

func TestInferCommandAuditFinalState(t *testing.T) {
	tests := map[string]string{
		"/amp iniciar":   "Online",
		"/amp parar":     "Idle",
		"/amp desligar":  "Offline",
		"/amp atualizar": "Atualização concluída",
		"/amp status":    "Painel atualizado",
	}

	for command, expected := range tests {
		actual := inferCommandAuditFinalState(
			command,
			"Operação concluída.",
			commandAuditPhaseCompleted,
		)
		if actual != expected {
			t.Fatalf("estado de %s: obtido %q, esperado %q", command, actual, expected)
		}
	}

	if actual := inferCommandAuditFinalState(
		"/amp iniciar",
		"Falhou",
		commandAuditPhaseFailed,
	); actual != "Não concluído" {
		t.Fatalf("estado de falha inesperado: %q", actual)
	}
}

func TestInferCommandAuditFinalStateAfterLeavingIdle(t *testing.T) {
	// Mensagens reais de sucesso do /amp iniciar (pt-BR e en-US).
	for _, result := range []string{
		"✅ O jogo de **Fabrica de Monstro** foi iniciado e saiu do modo Idle.",
		"✅ The game for **Fabrica de Monstro** was started and left Idle mode.",
	} {
		if actual := inferCommandAuditFinalState(
			"/amp iniciar",
			result,
			commandAuditPhaseCompleted,
		); actual != "Online" {
			t.Fatalf("estado após iniciar (%q): obtido %q, esperado \"Online\"", result, actual)
		}
	}

	if actual := inferCommandAuditFinalState(
		"/amp parar",
		"✅ Valheim entrou em modo Idle.",
		commandAuditPhaseCompleted,
	); actual != "Idle" {
		t.Fatalf("estado após parar: obtido %q, esperado \"Idle\"", actual)
	}
}

func TestFormatCommandAuditDuration(t *testing.T) {
	tests := map[time.Duration]string{
		250 * time.Millisecond:                    "menos de 1 s",
		12 * time.Second:                          "12 s",
		2*time.Minute + 18*time.Second:            "2 min 18 s",
		time.Hour + 3*time.Minute + 4*time.Second: "1 h 3 min 4 s",
	}

	for duration, expected := range tests {
		if actual := formatCommandAuditDuration(duration); actual != expected {
			t.Fatalf("duração %s: obtido %q, esperado %q", duration, actual, expected)
		}
	}
}

func TestCommandAuditOptionValueHandlesNonStringOptions(t *testing.T) {
	cases := []struct {
		option disgoDiscord.SlashCommandOption
		want   string
	}{
		{disgoDiscord.SlashCommandOption{Name: "minutos", Type: disgoDiscord.ApplicationCommandOptionTypeInt, Value: json.RawMessage(`20`)}, "20"},
		{disgoDiscord.SlashCommandOption{Name: "confirmar", Type: disgoDiscord.ApplicationCommandOptionTypeBool, Value: json.RawMessage(`true`)}, "true"},
		{disgoDiscord.SlashCommandOption{Name: "servidor", Type: disgoDiscord.ApplicationCommandOptionTypeString, Value: json.RawMessage(`"Valheim01"`)}, "Valheim01"},
	}
	for _, testCase := range cases {
		if got := commandAuditOptionValue(testCase.option); got != testCase.want {
			t.Fatalf("%s: got %q, want %q", testCase.option.Name, got, testCase.want)
		}
	}
}
