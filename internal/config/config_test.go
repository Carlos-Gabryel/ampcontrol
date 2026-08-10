package config

import "testing"

func TestNonNegativeEnvironmentInteger(t *testing.T) {
	const name = "AMPCONTROL_TEST_NON_NEGATIVE"

	t.Setenv(name, "")
	if value, err := nonNegativeEnvironmentInteger(name, 5); err != nil || value != 5 {
		t.Fatalf("valor padrão inesperado: valor=%d erro=%v", value, err)
	}

	t.Setenv(name, "0")
	if value, err := nonNegativeEnvironmentInteger(name, 5); err != nil || value != 0 {
		t.Fatalf("zero deveria desativar a configuração: valor=%d erro=%v", value, err)
	}

	t.Setenv(name, "12")
	if value, err := nonNegativeEnvironmentInteger(name, 5); err != nil || value != 12 {
		t.Fatalf("valor configurado inesperado: valor=%d erro=%v", value, err)
	}

	for _, invalid := range []string{"-1", "abc", "1.5"} {
		t.Setenv(name, invalid)
		if _, err := nonNegativeEnvironmentInteger(name, 5); err == nil {
			t.Fatalf("valor inválido %q deveria retornar erro", invalid)
		}
	}
}
