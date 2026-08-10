package discord

import (
	"testing"
	"time"
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
		{"✅ O jogo foi iniciado.", commandAuditPhaseCompleted, true},
		{"❌ Não foi possível iniciar.", commandAuditPhaseFailed, true},
		{"⛔ Canal não autorizado.", commandAuditPhaseRefused, true},
		{"⚠️ Confirmação ausente.", commandAuditPhaseFailed, true},
		{"⏳ O servidor já possui uma operação em andamento.", commandAuditPhaseFailed, true},
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
