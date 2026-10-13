package amp

import "testing"

func TestStripTerminalSequences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"vazio", "", ""},
		{"texto simples", "instância iniciada", "instância iniciada"},
		{"cor CSI", "\x1b[31mvermelho\x1b[0m", "vermelho"},
		{"cor CSI com vários parâmetros", "\x1b[1;38;5;196mforte\x1b[0m!", "forte!"},
		{"CSI sem parâmetros", "a\x1b[mb", "ab"},
		{"movimento do cursor", "a\x1b[2Ab\x1b[10;20Hc\x1b[3Dd", "abcd"},
		{"limpar linha e tela", "x\x1b[2K\x1b[2Jy", "xy"},
		{"CSI com sinal de interrogação", "a\x1b[?25lb\x1b[?25hc", "abc"},
		{"OSC terminado por BEL", "a\x1b]0;título da janela\x07b", "ab"},
		{"OSC terminado por ST", "a\x1b]8;;http://exemplo.com\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		{"OSC sem terminador descarta o resto", "a\x1b]0;sem fim", "a"},
		{"OSC com ESC solto no fim", "a\x1b]0;x\x1b", "a"},
		{"ESC solto no fim", "texto\x1b", "texto"},
		{"ESC seguido de caractere", "a\x1bMb", "ab"},
		{"ESC ESC", "a\x1b\x1bb", "ab"},
		{"CSI incompleto no fim", "a\x1b[31", "a"},
		{"CSI só com introdutor", "a\x1b[", "a"},
		{"unicode preservado", "ação → é 日本語 🎮", "ação → é 日本語 🎮"},
		{"caracteres de caixa preservados", "│ Nome │ ─┼─ ┌┐└┘ ║", "│ Nome │ ─┼─ ┌┐└┘ ║"},
		{"caixa e cor misturadas", "\x1b[32m│\x1b[0m ok \x1b[32m│\x1b[0m", "│ ok │"},
		{"quebras de linha e tabs preservados", "a\r\nb\tc\n", "a\r\nb\tc\n"},
		{"backspace preservado", "ab\bc", "ab\bc"},
		{"sequências consecutivas", "\x1b[1m\x1b[31m\x1b[4mx\x1b[0m", "x"},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := stripTerminalSequences(test.input); got != test.expected {
				t.Fatalf("obtido %q, esperado %q", got, test.expected)
			}
		})
	}
}
