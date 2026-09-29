package idle

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var jsonCodeBlockPattern = regexp.MustCompile("(?s)```json\n(.*?)```")

// Os exemplos de idle.json publicados precisam ser aceitos pelo carregador
// real; um exemplo com chave inexistente impede o serviço de iniciar.
func TestPublishedIdleExamplesLoad(t *testing.T) {
	examples := map[string]string{}

	content, err := os.ReadFile(filepath.Join("..", "..", "config", "idle.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	examples["config/idle.example.json"] = string(content)

	for _, document := range []string{"CONFIGURATION.md", "CONFIGURATION.en.md"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "docs", document))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, match := range jsonCodeBlockPattern.FindAllStringSubmatch(string(content), -1) {
			if strings.Contains(match[1], `"servers"`) {
				examples["docs/"+document] = match[1]
				found = true
			}
		}
		if !found {
			t.Fatalf("docs/%s não tem exemplo de idle.json", document)
		}
	}

	for name, example := range examples {
		path := filepath.Join(t.TempDir(), "idle.json")
		if err := os.WriteFile(path, []byte(example), 0o600); err != nil {
			t.Fatal(err)
		}
		config, err := Load(path)
		if err != nil {
			t.Fatalf("%s: exemplo rejeitado: %v", name, err)
		}
		for _, server := range config.Servers {
			if server.Enabled && server.Mode != ServerModeActive {
				t.Fatalf("%s: exemplo habilitado sem \"mode\": \"active\" nunca para o servidor", name)
			}
		}
	}
}
