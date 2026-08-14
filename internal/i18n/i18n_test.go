package i18n

import "testing"

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
