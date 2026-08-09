package idle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRegisterAdditionalServerPersistsActiveAMPDefaults(t *testing.T) {
	directory := t.TempDir()
	basePath := filepath.Join(directory, "idle.json")
	additionalPath := filepath.Join(directory, "idle_servers.json")
	base := `{
  "check_interval_seconds": 30,
  "default_idle_timeout_minutes": 15,
  "servers": [
    {
      "instance": "Valheim01",
      "display_name": "Valheim",
      "game": "Valheim",
      "enabled": true,
      "mode": "active",
      "detector": "amp_players"
    }
  ]
}`
	if err := os.WriteFile(basePath, []byte(base), 0o600); err != nil {
		t.Fatalf("não foi possível preparar a configuração base: %v", err)
	}

	config, err := RegisterAdditionalServer(
		basePath,
		additionalPath,
		ServerRegistration{
			Instance:    "NovoServidor01",
			DisplayName: "Novo Servidor",
			Game:        "Minecraft",
		},
	)
	if err != nil {
		t.Fatalf("RegisterAdditionalServer retornou erro: %v", err)
	}

	server, exists := config.FindServer("novoservidor01")
	if !exists {
		t.Fatal("o servidor adicional não foi mesclado")
	}
	if !server.IsActive() || server.Detector != DetectorAMPPlayers {
		t.Fatalf("cadastro adicional inesperado: %+v", server)
	}
	if server.IdleTimeout != 15*time.Minute ||
		server.StartupGrace != 5*time.Minute {
		t.Fatalf("tempos padrão inesperados: %+v", server)
	}

	reloaded, err := LoadCombined(basePath, additionalPath)
	if err != nil {
		t.Fatalf("não foi possível recarregar a configuração combinada: %v", err)
	}
	if len(reloaded.Servers) != 2 {
		t.Fatalf("quantidade recarregada inesperada: %d", len(reloaded.Servers))
	}
	info, err := os.Stat(additionalPath)
	if err != nil {
		t.Fatalf("arquivo adicional não foi criado: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("modo inseguro do arquivo adicional: %o", info.Mode().Perm())
	}

	_, err = RegisterAdditionalServer(
		basePath,
		additionalPath,
		ServerRegistration{Instance: "NOVOSERVIDOR01"},
	)
	if !errors.Is(err, ErrServerAlreadyRegistered) {
		t.Fatalf("era esperado ErrServerAlreadyRegistered, recebido: %v", err)
	}
}

type ampPlayersEngineTestDetector struct{}

func (ampPlayersEngineTestDetector) DetectorType() Detector {
	return DetectorAMPPlayers
}

func (ampPlayersEngineTestDetector) PlayerCount(
	context.Context,
	Server,
) (int, error) {
	return 0, nil
}

func TestEngineReplaceConfigAddsServerWithoutRestart(t *testing.T) {
	registry, err := NewDetectorRegistry(
		&scriptedEngineDetector{},
		ampPlayersEngineTestDetector{},
	)
	if err != nil {
		t.Fatalf("não foi possível criar o registro: %v", err)
	}

	base := engineTestConfig()
	engine, err := NewEngine(
		base,
		registry,
		&scriptedRuntimeProvider{defaultState: RuntimeStateOffline},
		&fakeApplicationStopper{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewEngine retornou erro: %v", err)
	}

	next := Config{
		CheckInterval: base.CheckInterval,
		Servers: append(append([]Server(nil), base.Servers...), Server{
			Instance:     "NovoServidor01",
			DisplayName:  "Novo Servidor",
			Enabled:      true,
			Mode:         ServerModeActive,
			Detector:     DetectorAMPPlayers,
			IdleTimeout:  15 * time.Minute,
			StartupGrace: 5 * time.Minute,
		}),
	}
	if err := engine.ReplaceConfig(next); err != nil {
		t.Fatalf("ReplaceConfig retornou erro: %v", err)
	}

	events := engine.CheckNow(context.Background())
	if len(events) != 2 {
		t.Fatalf("o ciclo deveria observar dois servidores, recebeu %d eventos", len(events))
	}
}
