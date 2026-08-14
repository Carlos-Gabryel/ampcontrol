package i18n

import (
	"strings"
	"testing"
)

func TestCatalogsContainTheSameKeys(t *testing.T) {
	portuguese := catalogs[PortugueseBrazil]
	english := catalogs[EnglishUS]
	if len(portuguese) != len(english) {
		t.Fatalf("catalog sizes differ: pt-BR=%d en-US=%d", len(portuguese), len(english))
	}
	for key := range portuguese {
		if english[key] == "" {
			t.Errorf("English translation missing for %q", key)
		}
	}
}

func TestParseLanguageAliases(t *testing.T) {
	for input, expected := range map[string]Language{
		"":      PortugueseBrazil,
		"pt-BR": PortugueseBrazil,
		"pt":    PortugueseBrazil,
		"en-US": EnglishUS,
		"en":    EnglishUS,
	} {
		actual, err := Parse(input)
		if err != nil || actual != expected {
			t.Fatalf("Parse(%q) = %q, %v; want %q", input, actual, err, expected)
		}
	}
	if _, err := Parse("fr"); err == nil {
		t.Fatal("unsupported language should fail")
	}
}

func TestRuntimeTextUsesSelectedLanguage(t *testing.T) {
	previous := Default()
	t.Cleanup(func() { SetDefault(previous) })

	SetDefault(PortugueseBrazil)
	if got := Text("⚠️ Nenhum subcomando foi informado."); got != "⚠️ Nenhum subcomando foi informado." {
		t.Fatalf("Portuguese text changed: %q", got)
	}

	SetDefault(EnglishUS)
	if got := Text("⚠️ Nenhum subcomando foi informado."); got != "⚠️ No subcommand was provided." {
		t.Fatalf("English translation = %q", got)
	}
	if got := Text("⏱️ Aguarde **10 s** antes de enviar outro comando para a instância `demo`."); strings.Contains(got, "Aguarde") || strings.Contains(got, "instância") {
		t.Fatalf("dynamic text was not translated: %q", got)
	}
}
