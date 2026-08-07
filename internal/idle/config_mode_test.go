package idle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultsServerModeToObserve(
	t *testing.T,
) {
	t.Parallel()

	path := writeIdleModeTestConfig(
		t,
		`{
			"check_interval_seconds": 30,
			"default_idle_timeout_minutes": 15,
			"servers": [
				{
					"instance": "AlamamaPal01",
					"display_name": "Alamama",
					"enabled": true,
					"detector": "palworld_rcon",
					"idle_timeout_minutes": 15,
					"startup_grace_minutes": 5,
					"rcon": {
						"address": "127.0.0.1:25575",
						"password_env": "ALAMAMA_RCON_PASSWORD"
					}
				}
			]
		}`,
	)

	config, err := Load(
		path,
	)
	if err != nil {
		t.Fatalf(
			"Load retornou erro: %v",
			err,
		)
	}

	server, exists := config.FindServer(
		"AlamamaPal01",
	)
	if !exists {
		t.Fatal(
			"AlamamaPal01 não foi encontrada",
		)
	}

	if server.Mode != ServerModeObserve {
		t.Fatalf(
			"modo padrão inesperado: %q",
			server.Mode,
		)
	}

	if server.IsActive() {
		t.Fatal(
			"servidor sem mode explícito não deveria ficar ativo",
		)
	}

	if len(config.ActiveServers()) != 0 {
		t.Fatalf(
			"nenhum servidor ativo era esperado; encontrados: %d",
			len(config.ActiveServers()),
		)
	}
}

func TestLoadAcceptsActiveServerMode(
	t *testing.T,
) {
	t.Parallel()

	path := writeIdleModeTestConfig(
		t,
		`{
			"check_interval_seconds": 30,
			"default_idle_timeout_minutes": 15,
			"servers": [
				{
					"instance": "AlamamaPal01",
					"display_name": "Alamama",
					"enabled": true,
					"mode": "active",
					"detector": "palworld_rcon",
					"idle_timeout_minutes": 15,
					"startup_grace_minutes": 5,
					"rcon": {
						"address": "127.0.0.1:25575",
						"password_env": "ALAMAMA_RCON_PASSWORD"
					}
				},
				{
					"instance": "AIO01",
					"display_name": "AIO",
					"enabled": false,
					"mode": "active"
				}
			]
		}`,
	)

	config, err := Load(
		path,
	)
	if err != nil {
		t.Fatalf(
			"Load retornou erro: %v",
			err,
		)
	}

	alamama, exists := config.FindServer(
		"AlamamaPal01",
	)
	if !exists {
		t.Fatal(
			"AlamamaPal01 não foi encontrada",
		)
	}

	if alamama.Mode != ServerModeActive {
		t.Fatalf(
			"modo inesperado para Alamama: %q",
			alamama.Mode,
		)
	}

	if !alamama.IsActive() {
		t.Fatal(
			"Alamama deveria estar ativa",
		)
	}

	aio, exists := config.FindServer(
		"AIO01",
	)
	if !exists {
		t.Fatal(
			"AIO01 não foi encontrada",
		)
	}

	if aio.Mode != ServerModeActive {
		t.Fatalf(
			"modo inesperado para AIO01: %q",
			aio.Mode,
		)
	}

	if aio.IsActive() {
		t.Fatal(
			"servidor disabled não pode ser considerado ativo",
		)
	}

	activeServers := config.ActiveServers()

	if len(activeServers) != 1 {
		t.Fatalf(
			"era esperado exatamente um servidor ativo; encontrados: %d",
			len(activeServers),
		)
	}

	if activeServers[0].Instance != "AlamamaPal01" {
		t.Fatalf(
			"servidor ativo inesperado: %s",
			activeServers[0].Instance,
		)
	}
}

func TestLoadRejectsUnknownServerMode(
	t *testing.T,
) {
	t.Parallel()

	path := writeIdleModeTestConfig(
		t,
		`{
			"check_interval_seconds": 30,
			"default_idle_timeout_minutes": 15,
			"servers": [
				{
					"instance": "AlamamaPal01",
					"display_name": "Alamama",
					"enabled": true,
					"mode": "qualquer_coisa",
					"detector": "palworld_rcon",
					"idle_timeout_minutes": 15,
					"startup_grace_minutes": 5,
					"rcon": {
						"address": "127.0.0.1:25575",
						"password_env": "ALAMAMA_RCON_PASSWORD"
					}
				}
			]
		}`,
	)

	_, err := Load(
		path,
	)

	if err == nil {
		t.Fatal(
			"era esperado erro para modo de Idle desconhecido",
		)
	}

	if !strings.Contains(
		err.Error(),
		"modo de Idle",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"qualquer_coisa",
	) {
		t.Fatalf(
			"o erro deveria informar o modo inválido: %v",
			err,
		)
	}
}

func writeIdleModeTestConfig(
	t *testing.T,
	content string,
) string {
	t.Helper()

	path := filepath.Join(
		t.TempDir(),
		"idle.json",
	)

	if err := os.WriteFile(
		path,
		[]byte(content),
		0o600,
	); err != nil {
		t.Fatalf(
			"não foi possível criar configuração de teste: %v",
			err,
		)
	}

	return path
}
