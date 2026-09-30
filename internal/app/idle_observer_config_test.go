package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/idle"
	"github.com/Carlos-Gabryel/ampcontrol/internal/operation"
)

const idleConfigTestFixture = `{
  "check_interval_seconds": 20,
  "default_idle_timeout_minutes": 10,
  "servers": [
    {
      "instance": "Pal01",
      "display_name": "Palworld",
      "game": "Palworld",
      "enabled": true,
      "mode": "active",
      "detector": "amp_players",
      "fallback_detector": "palworld_rcon",
      "rcon": {
        "address": "127.0.0.1:25575",
        "password_env": "TEST_PAL01_RCON_PASSWORD"
      }
    },
    {
      "instance": "Zomb01",
      "display_name": "Zomboid",
      "game": "Project Zomboid",
      "enabled": true,
      "mode": "active",
      "detector": "amp_players",
      "fallback_detector": "project_zomboid_rcon",
      "rcon": {
        "address": "127.0.0.1:27015",
        "password_env": "TEST_ZOMB01_RCON_PASSWORD"
      }
    }
  ]
}
`

type emptyInventory struct{}

func (emptyInventory) DiscoverInstances(context.Context) ([]amp.ManagedInstance, error) {
	return nil, nil
}

// newConfigTestObserver muda para um diretório temporário (os caminhos do
// observador são relativos), grava config/idle.json e cria o observador.
func newConfigTestObserver(t *testing.T) (*idleObserver, []byte) {
	t.Helper()
	t.Chdir(t.TempDir())

	if err := os.MkdirAll("config", 0o750); err != nil {
		t.Fatalf("não foi possível criar config/: %v", err)
	}
	if err := os.MkdirAll("data", 0o750); err != nil {
		t.Fatalf("não foi possível criar data/: %v", err)
	}
	if err := os.WriteFile(idleObserverConfigPath, []byte(idleConfigTestFixture), 0o600); err != nil {
		t.Fatalf("não foi possível gravar idle.json: %v", err)
	}

	cfg, err := idle.Load(idleObserverConfigPath)
	if err != nil {
		t.Fatalf("idle.Load falhou: %v", err)
	}

	observer, err := newIdleObserver(
		amp.NewAPIClient("usuario-teste", "senha-teste"),
		emptyInventory{},
		operation.NewManager(),
		noopIdleNotifier{},
		cfg,
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf("newIdleObserver falhou: %v", err)
	}

	original, err := os.ReadFile(idleObserverConfigPath)
	if err != nil {
		t.Fatalf("não foi possível ler idle.json: %v", err)
	}
	return observer, original
}

// engineConfig lê a configuração em uso pelo motor para que os testes
// enxerguem também o rollback do motor, não só o snapshot.
func engineConfig(t *testing.T, o *idleObserver) idle.Config {
	t.Helper()
	return o.engine.Config()
}

func findTestServer(t *testing.T, cfg idle.Config, instance string) idle.Server {
	t.Helper()
	server, ok := cfg.FindServer(instance)
	if !ok {
		t.Fatalf("a instância %s não foi encontrada na configuração", instance)
	}
	return server
}

func assertIdleJSONUntouched(t *testing.T, original []byte) {
	t.Helper()
	current, err := os.ReadFile(idleObserverConfigPath)
	if err != nil {
		t.Fatalf("não foi possível reler idle.json: %v", err)
	}
	if !bytes.Equal(current, original) {
		t.Fatalf("idle.json foi alterado; o bot nunca deve gravá-lo:\n%s", current)
	}
}

func assertNoOverridesFile(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(idleDetectionOverridesPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("o arquivo de overrides não deveria existir, erro do stat: %v", err)
	}
}

func TestRegisterIdleServerPersistsWithoutTouchingIdleJSON(t *testing.T) {
	observer, original := newConfigTestObserver(t)

	calls := 0
	var received idle.Config
	observer.SetConfigChangedHandler(func(cfg idle.Config) {
		calls++
		received = cfg
	})

	err := observer.RegisterIdleServer(context.Background(), amp.ManagedInstance{
		Name:         "Nova01",
		FriendlyName: "Servidor Novo",
		Game:         "Minecraft",
	})
	if err != nil {
		t.Fatalf("RegisterIdleServer falhou: %v", err)
	}

	assertIdleJSONUntouched(t, original)

	data, err := os.ReadFile(idleAdditionalServersPath)
	if err != nil {
		t.Fatalf("idle_servers.json não foi criado: %v", err)
	}
	if !strings.Contains(string(data), "Nova01") {
		t.Fatalf("idle_servers.json não contém a instância registrada:\n%s", data)
	}

	snapshot := observer.configSnapshot()
	if len(snapshot.Servers) != 3 {
		t.Fatalf("o snapshot deveria ter 3 servidores, obteve %d", len(snapshot.Servers))
	}
	added := findTestServer(t, snapshot, "Nova01")
	if added.DisplayName != "Servidor Novo" || added.Game != "Minecraft" {
		t.Fatalf("servidor registrado com dados inesperados: %+v", added)
	}
	findTestServer(t, snapshot, "Pal01")

	if got := len(engineConfig(t, observer).Servers); got != 3 {
		t.Fatalf("o motor deveria ter 3 servidores, obteve %d", got)
	}

	if calls != 1 {
		t.Fatalf("o handler de mudança deveria ser chamado 1 vez, foi %d", calls)
	}
	if len(received.Servers) != 3 {
		t.Fatalf("o handler recebeu %d servidores, esperado 3", len(received.Servers))
	}
}

func TestRegisterIdleServerDuplicateReturnsError(t *testing.T) {
	observer, original := newConfigTestObserver(t)

	calls := 0
	observer.SetConfigChangedHandler(func(idle.Config) { calls++ })

	err := observer.RegisterIdleServer(context.Background(), amp.ManagedInstance{
		Name: "pal01",
		Game: "Palworld",
	})
	if !errors.Is(err, idle.ErrServerAlreadyRegistered) {
		t.Fatalf("esperado ErrServerAlreadyRegistered, obteve: %v", err)
	}

	assertIdleJSONUntouched(t, original)
	if _, statErr := os.Stat(idleAdditionalServersPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("idle_servers.json não deveria ter sido criado, stat: %v", statErr)
	}
	if len(observer.configSnapshot().Servers) != 2 {
		t.Fatal("o snapshot não deveria ter mudado")
	}
	if calls != 0 {
		t.Fatalf("o handler não deveria ser chamado, foi %d vezes", calls)
	}
}

func TestSetIdleDetectionMethodPersistsOverride(t *testing.T) {
	observer, original := newConfigTestObserver(t)

	calls := 0
	observer.SetConfigChangedHandler(func(idle.Config) { calls++ })

	settings, err := observer.SetIdleDetectionMethod(
		context.Background(),
		"Pal01",
		"amp_project_zomboid_rcon",
	)
	if err != nil {
		t.Fatalf("SetIdleDetectionMethod falhou: %v", err)
	}

	if settings.Method != "amp_project_zomboid_rcon" ||
		settings.Detector != string(idle.DetectorAMPPlayers) ||
		settings.FallbackDetector != string(idle.DetectorProjectZomboidRCON) ||
		!settings.RCONConfigured {
		t.Fatalf("configurações retornadas inesperadas: %+v", settings)
	}

	assertIdleJSONUntouched(t, original)

	overrides, err := idle.LoadDetectionOverrides(idleDetectionOverridesPath)
	if err != nil {
		t.Fatalf("não foi possível ler os overrides: %v", err)
	}
	if len(overrides) != 1 ||
		overrides[0].Instance != "Pal01" ||
		overrides[0].FallbackDetector != idle.DetectorProjectZomboidRCON {
		t.Fatalf("overrides inesperados: %+v", overrides)
	}

	server := findTestServer(t, observer.configSnapshot(), "Pal01")
	if server.Detector != idle.DetectorAMPPlayers ||
		server.FallbackDetector != idle.DetectorProjectZomboidRCON {
		t.Fatalf("snapshot com detectores inesperados: %+v", server)
	}
	engineServer := findTestServer(t, engineConfig(t, observer), "Pal01")
	if engineServer.FallbackDetector != idle.DetectorProjectZomboidRCON {
		t.Fatalf("o motor não recebeu o fallback: %+v", engineServer)
	}
	if calls != 1 {
		t.Fatalf("o handler deveria ser chamado 1 vez, foi %d", calls)
	}
}

func TestSetIdleDetectionMethodRejectsInvalidMethod(t *testing.T) {
	observer, original := newConfigTestObserver(t)
	before := observer.configSnapshot()

	_, err := observer.SetIdleDetectionMethod(context.Background(), "Pal01", "telepatia")
	if err == nil {
		t.Fatal("era esperado erro para método de detecção inválido")
	}

	assertNoOverridesFile(t)
	assertIdleJSONUntouched(t, original)
	if !reflect.DeepEqual(before, observer.configSnapshot()) {
		t.Fatal("o snapshot não deveria ter mudado")
	}
}

func TestSetIdleDetectionMethodRejectsUnknownInstanceAndMissingRCON(t *testing.T) {
	observer, _ := newConfigTestObserver(t)

	if _, err := observer.SetIdleDetectionMethod(context.Background(), "Inexistente", "amp"); err == nil {
		t.Fatal("era esperado erro para instância não cadastrada")
	}

	// Instância registrada em tempo real não tem RCON configurado.
	if err := observer.RegisterIdleServer(context.Background(), amp.ManagedInstance{Name: "Nova01"}); err != nil {
		t.Fatalf("RegisterIdleServer falhou: %v", err)
	}
	before := observer.configSnapshot()

	if _, err := observer.SetIdleDetectionMethod(context.Background(), "Nova01", "amp_palworld_rcon"); err == nil {
		t.Fatal("era esperado erro ao ativar RCON sem endereço/credencial")
	}

	assertNoOverridesFile(t)
	if !reflect.DeepEqual(before, observer.configSnapshot()) {
		t.Fatal("o snapshot não deveria ter mudado")
	}
	if len(engineConfig(t, observer).Servers) != 3 {
		t.Fatal("o motor não deveria ter mudado")
	}
}

func TestSetIdleDetectionMethodRollsBackWhenPersistFails(t *testing.T) {
	observer, original := newConfigTestObserver(t)

	// Um diretório no lugar do arquivo faz a persistência falhar.
	if err := os.Mkdir(idleDetectionOverridesPath, 0o750); err != nil {
		t.Fatalf("não foi possível criar o diretório bloqueador: %v", err)
	}

	calls := 0
	observer.SetConfigChangedHandler(func(idle.Config) { calls++ })

	_, err := observer.SetIdleDetectionMethod(context.Background(), "Pal01", "amp_project_zomboid_rcon")
	if err == nil {
		t.Fatal("era esperado erro quando a persistência falha")
	}

	assertIdleJSONUntouched(t, original)

	snapshotServer := findTestServer(t, observer.configSnapshot(), "Pal01")
	if snapshotServer.FallbackDetector != idle.DetectorPalworldRCON {
		t.Fatalf("o snapshot deveria manter o fallback anterior, obteve %q", snapshotServer.FallbackDetector)
	}
	engineServer := findTestServer(t, engineConfig(t, observer), "Pal01")
	if engineServer.FallbackDetector != idle.DetectorPalworldRCON {
		t.Fatalf("o motor deveria ter sofrido rollback, fallback atual %q", engineServer.FallbackDetector)
	}
	if calls != 0 {
		t.Fatalf("o handler não deveria ser chamado, foi %d vezes", calls)
	}
}

func TestResetIdleDetectionMethodRemovesOnlyThatOverride(t *testing.T) {
	observer, original := newConfigTestObserver(t)
	ctx := context.Background()

	if _, err := observer.SetIdleDetectionMethod(ctx, "Pal01", "amp"); err != nil {
		t.Fatalf("override de Pal01 falhou: %v", err)
	}
	if _, err := observer.SetIdleDetectionMethod(ctx, "Zomb01", "amp"); err != nil {
		t.Fatalf("override de Zomb01 falhou: %v", err)
	}

	calls := 0
	observer.SetConfigChangedHandler(func(idle.Config) { calls++ })

	settings, err := observer.ResetIdleDetectionMethod(ctx, "Pal01")
	if err != nil {
		t.Fatalf("ResetIdleDetectionMethod falhou: %v", err)
	}
	if settings.Method != "amp_palworld_rcon" || settings.FallbackDetector != string(idle.DetectorPalworldRCON) {
		t.Fatalf("configurações após reset inesperadas: %+v", settings)
	}

	assertIdleJSONUntouched(t, original)

	overrides, err := idle.LoadDetectionOverrides(idleDetectionOverridesPath)
	if err != nil {
		t.Fatalf("não foi possível ler os overrides: %v", err)
	}
	if len(overrides) != 1 || overrides[0].Instance != "Zomb01" {
		t.Fatalf("apenas o override de Zomb01 deveria restar: %+v", overrides)
	}

	snapshot := observer.configSnapshot()
	if got := findTestServer(t, snapshot, "Pal01").FallbackDetector; got != idle.DetectorPalworldRCON {
		t.Fatalf("Pal01 deveria voltar ao fallback base, obteve %q", got)
	}
	if got := findTestServer(t, snapshot, "Zomb01").FallbackDetector; got != "" {
		t.Fatalf("Zomb01 deveria manter o override, fallback %q", got)
	}
	engine := engineConfig(t, observer)
	if got := findTestServer(t, engine, "Pal01").FallbackDetector; got != idle.DetectorPalworldRCON {
		t.Fatalf("o motor deveria ter Pal01 no detector base, fallback %q", got)
	}
	if got := findTestServer(t, engine, "Zomb01").FallbackDetector; got != "" {
		t.Fatalf("o motor deveria manter Zomb01, fallback %q", got)
	}
	if calls != 1 {
		t.Fatalf("o handler deveria ser chamado 1 vez, foi %d", calls)
	}
}

func TestResetAndSettingsRejectUnregisteredInstance(t *testing.T) {
	observer, _ := newConfigTestObserver(t)

	if _, err := observer.ResetIdleDetectionMethod(context.Background(), "Inexistente"); err == nil {
		t.Fatal("era esperado erro no reset de instância não cadastrada")
	}
	if _, err := observer.IdleDetectionSettings("Inexistente"); err == nil {
		t.Fatal("era esperado erro nas configurações de instância não cadastrada")
	}
	assertNoOverridesFile(t)
}

func TestIdleConfigMutationsRequireEngine(t *testing.T) {
	ctx := context.Background()

	for name, observer := range map[string]*idleObserver{
		"nil":          nil,
		"sem motor":    {},
		"config vazia": {config: idle.Config{}},
	} {
		if err := observer.RegisterIdleServer(ctx, amp.ManagedInstance{Name: "X"}); err == nil {
			t.Fatalf("%s: RegisterIdleServer deveria retornar erro", name)
		}
		if _, err := observer.SetIdleDetectionMethod(ctx, "X", "amp"); err == nil {
			t.Fatalf("%s: SetIdleDetectionMethod deveria retornar erro", name)
		}
		if _, err := observer.ResetIdleDetectionMethod(ctx, "X"); err == nil {
			t.Fatalf("%s: ResetIdleDetectionMethod deveria retornar erro", name)
		}
	}
}
