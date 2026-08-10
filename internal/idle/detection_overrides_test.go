package idle

import (
	"path/filepath"
	"testing"
)

func detectionOverrideTestConfig() Config {
	return Config{
		CheckInterval: defaultCheckIntervalSeconds,
		Servers: []Server{
			{
				Instance:     "Valheim01",
				Enabled:      true,
				Mode:         ServerModeActive,
				Detector:     DetectorAMPPlayers,
				IdleTimeout:  defaultIdleTimeoutMinutes,
				StartupGrace: defaultStartupGraceMinutes,
			},
			{
				Instance:        "TheWalkingRats01",
				Enabled:         true,
				Mode:            ServerModeActive,
				Detector:        DetectorAMPPlayers,
				IdleTimeout:     defaultIdleTimeoutMinutes,
				StartupGrace:    defaultStartupGraceMinutes,
				RCONAddress:     "127.0.0.1:27015",
				RCONPasswordEnv: "PZ_RCON_PASSWORD",
			},
		},
	}
}

func TestDetectionOverridesRoundTripAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "idle_detection_overrides.json")
	override := DetectionOverride{
		Instance:         "TheWalkingRats01",
		Detector:         DetectorAMPPlayers,
		FallbackDetector: DetectorProjectZomboidRCON,
	}
	if err := SetDetectionOverride(path, override); err != nil {
		t.Fatalf("não foi possível salvar o override: %v", err)
	}

	loaded, err := LoadDetectionOverrides(path)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("overrides recarregados inesperados: %#v err=%v", loaded, err)
	}
	next, err := ApplyDetectionOverrides(detectionOverrideTestConfig(), loaded)
	if err != nil {
		t.Fatalf("não foi possível aplicar o override: %v", err)
	}
	server, _ := next.FindServer("thewalkingrats01")
	if server.FallbackDetector != DetectorProjectZomboidRCON {
		t.Fatalf("fallback inesperado: %q", server.FallbackDetector)
	}

	removed, err := RemoveDetectionOverride(path, "THEWALKINGRATS01")
	if err != nil || !removed {
		t.Fatalf("não foi possível remover o override: removed=%v err=%v", removed, err)
	}
}

func TestDetectionOverrideRejectsRCONWithoutConfiguration(t *testing.T) {
	_, err := ApplyDetectionOverrides(
		detectionOverrideTestConfig(),
		[]DetectionOverride{{
			Instance:         "Valheim01",
			Detector:         DetectorAMPPlayers,
			FallbackDetector: DetectorPalworldRCON,
		}},
	)
	if err == nil {
		t.Fatal("RCON sem endereço e variável de senha deveria ser rejeitado")
	}
}

func TestDetectionOverrideRejectsUnknownInstance(t *testing.T) {
	_, err := ApplyDetectionOverrides(
		detectionOverrideTestConfig(),
		[]DetectionOverride{{
			Instance: "Missing01",
			Detector: DetectorAMPPlayers,
		}},
	)
	if err == nil {
		t.Fatal("override de instância inexistente deveria ser rejeitado")
	}
}
