package idle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadValidConfig(
	t *testing.T,
) {
	t.Parallel()

	path := writeTestConfig(
		t,
		`{
			"check_interval_seconds": 20,
			"default_idle_timeout_minutes": 12,
			"servers": [
				{
					"instance": "AlamamaPal01",
					"display_name": "Alamama",
					"enabled": true,
					"detector": "palworld_rcon",
					"startup_grace_minutes": 4,
					"rcon": {
						"address": "127.0.0.1:25575",
						"password_env": "TEST_ALAMAMA_RCON_PASSWORD"
					}
				},
				{
					"instance": "Valheim01",
					"display_name": "Valheim",
					"enabled": false
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

	if config.CheckInterval != 20*time.Second {
		t.Fatalf(
			"intervalo inesperado: %s",
			config.CheckInterval,
		)
	}

	if len(config.Servers) != 2 {
		t.Fatalf(
			"quantidade inesperada de servidores: %d",
			len(config.Servers),
		)
	}

	enabled := config.EnabledServers()

	if len(enabled) != 1 {
		t.Fatalf(
			"quantidade inesperada de servidores habilitados: %d",
			len(enabled),
		)
	}

	server := enabled[0]

	if server.Instance != "AlamamaPal01" {
		t.Fatalf(
			"instância inesperada: %q",
			server.Instance,
		)
	}

	if server.Detector != DetectorPalworldRCON {
		t.Fatalf(
			"detector inesperado: %q",
			server.Detector,
		)
	}

	if server.IdleTimeout != 12*time.Minute {
		t.Fatalf(
			"tempo de Idle inesperado: %s",
			server.IdleTimeout,
		)
	}

	if server.StartupGrace != 4*time.Minute {
		t.Fatalf(
			"tempo de proteção inicial inesperado: %s",
			server.StartupGrace,
		)
	}

	if server.RCONAddress != "127.0.0.1:25575" {
		t.Fatalf(
			"endereço RCON inesperado: %q",
			server.RCONAddress,
		)
	}
}

func TestLoadRejectsDuplicateInstances(
	t *testing.T,
) {
	t.Parallel()

	path := writeTestConfig(
		t,
		`{
			"servers": [
				{
					"instance": "AIO01",
					"enabled": false
				},
				{
					"instance": "aio01",
					"enabled": false
				}
			]
		}`,
	)

	_, err := Load(
		path,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para instâncias duplicadas",
		)
	}

	if !strings.Contains(
		err.Error(),
		"aparece mais de uma vez",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestLoadRejectsEnabledServerWithoutDetector(
	t *testing.T,
) {
	t.Parallel()

	path := writeTestConfig(
		t,
		`{
			"servers": [
				{
					"instance": "AlamamaPal01",
					"enabled": true
				}
			]
		}`,
	)

	_, err := Load(
		path,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para servidor habilitado sem detector",
		)
	}

	if !strings.Contains(
		err.Error(),
		"nenhum detector foi informado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDisabledServerCanRemainWithoutDetector(
	t *testing.T,
) {
	t.Parallel()

	path := writeTestConfig(
		t,
		`{
			"servers": [
				{
					"instance": "Valheim01",
					"display_name": "Valheim",
					"enabled": false
				}
			]
		}`,
	)

	config, err := Load(
		path,
	)
	if err != nil {
		t.Fatalf(
			"servidor desabilitado deveria ser aceito: %v",
			err,
		)
	}

	if len(config.EnabledServers()) != 0 {
		t.Fatal(
			"nenhum servidor deveria estar habilitado",
		)
	}
}

func TestLoadRejectsUnknownField(
	t *testing.T,
) {
	t.Parallel()

	path := writeTestConfig(
		t,
		`{
			"campo_inexistente": true,
			"servers": [
				{
					"instance": "Valheim01",
					"enabled": false
				}
			]
		}`,
	)

	_, err := Load(
		path,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para campo desconhecido",
		)
	}

	if !strings.Contains(
		err.Error(),
		"unknown field",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestRCONPasswordUsesEnvironmentVariable(
	t *testing.T,
) {
	t.Setenv(
		"TEST_RCON_SECRET",
		"senha-de-teste",
	)

	server := Server{
		Instance:        "ServidorTeste01",
		RCONPasswordEnv: "TEST_RCON_SECRET",
	}

	password, err := server.RCONPassword()
	if err != nil {
		t.Fatalf(
			"RCONPassword retornou erro: %v",
			err,
		)
	}

	if password != "senha-de-teste" {
		t.Fatalf(
			"senha inesperada: %q",
			password,
		)
	}
}

func TestRCONPasswordRejectsMissingEnvironmentVariable(
	t *testing.T,
) {
	server := Server{
		Instance:        "ServidorTeste01",
		RCONPasswordEnv: "VARIAVEL_QUE_NAO_EXISTE_NO_TESTE",
	}

	_, err := server.RCONPassword()

	if err == nil {
		t.Fatal(
			"era esperado um erro para variável ausente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não foi configurada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestFindServer(
	t *testing.T,
) {
	t.Parallel()

	config := Config{
		Servers: []Server{
			{
				Instance:    "AlamamaPal01",
				DisplayName: "Alamama",
			},
		},
	}

	server, exists := config.FindServer(
		"alamamapal01",
	)

	if !exists {
		t.Fatal(
			"a instância deveria ter sido encontrada",
		)
	}

	if server.DisplayName != "Alamama" {
		t.Fatalf(
			"nome amigável inesperado: %q",
			server.DisplayName,
		)
	}
}

func TestProjectIdleConfigLoads(
	t *testing.T,
) {
	t.Parallel()

	path := filepath.Join(
		"..",
		"..",
		"config",
		"idle.json",
	)

	config, err := Load(
		path,
	)
	if err != nil {
		t.Fatalf(
			"a configuração real de Idle é inválida: %v",
			err,
		)
	}

	if len(config.Servers) != 11 {
		t.Fatalf(
			"eram esperados 11 servidores, mas foram encontrados %d",
			len(config.Servers),
		)
	}

	enabled := config.EnabledServers()

	if len(enabled) != 11 {
		t.Fatalf(
			"eram esperados 11 servidores habilitados, mas foram encontrados %d",
			len(enabled),
		)
	}

	active := config.ActiveServers()
	if len(active) != 2 {
		t.Fatalf(
			"eram esperados 2 servidores ativos, mas foram encontrados %d",
			len(active),
		)
	}

	alamama, exists := config.FindServer("AlamamaPal01")
	if !exists {
		t.Fatal("AlamamaPal01 não foi encontrada")
	}
	if alamama.Detector != DetectorAMPPlayers ||
		alamama.FallbackDetector != DetectorPalworldRCON {
		t.Fatalf(
			"cadeia de detectores inesperada: primário=%q fallback=%q",
			alamama.Detector,
			alamama.FallbackDetector,
		)
	}
	if alamama.Game != "Palworld" {
		t.Fatalf("jogo inesperado: %q", alamama.Game)
	}
}

func writeTestConfig(
	t *testing.T,
	content string,
) string {
	t.Helper()

	directory := t.TempDir()

	path := filepath.Join(
		directory,
		"idle.json",
	)

	err := os.WriteFile(
		path,
		[]byte(content),
		0600,
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar a configuração de teste: %v",
			err,
		)
	}

	return path
}
