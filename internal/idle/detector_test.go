package idle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakePlayerDetector struct {
	detectorType Detector
	playerCount  int
	err          error
	calls        int
}

func (d *fakePlayerDetector) DetectorType() Detector {
	return d.detectorType
}

func (d *fakePlayerDetector) PlayerCount(
	context.Context,
	Server,
) (int, error) {
	d.calls++

	return d.playerCount, d.err
}

func TestNewDefaultDetectorRegistrySupportsPalworld(
	t *testing.T,
) {
	t.Parallel()

	registry, err := NewDefaultDetectorRegistry()
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro padrão: %v",
			err,
		)
	}

	if !registry.Supports(DetectorPalworldRCON) {
		t.Fatal(
			"o registro padrão deveria possuir palworld_rcon",
		)
	}

	if !registry.Supports(DetectorProjectZomboidRCON) {
		t.Fatal(
			"o registro padrão deveria possuir project_zomboid_rcon",
		)
	}

	if registry.Supports(DetectorMinecraftRCON) {
		t.Fatal(
			"minecraft_rcon ainda não deveria estar implementado",
		)
	}
}

func TestProjectZomboidRCONDetectorUsesEnvironmentVariable(
	t *testing.T,
) {
	t.Setenv(
		"TEST_PROJECT_ZOMBOID_RCON_PASSWORD",
		"senha-de-teste",
	)

	called := false
	detector := ProjectZomboidRCONDetector{
		playerCount: func(
			_ context.Context,
			address string,
			password string,
		) (int, error) {
			called = true
			if address != "127.0.0.1:27015" {
				t.Fatalf("endereço inesperado: %q", address)
			}
			if password != "senha-de-teste" {
				t.Fatalf("senha inesperada")
			}

			return 2, nil
		},
	}

	count, err := detector.PlayerCount(
		context.Background(),
		Server{
			Instance:        "TheWalkingRats01",
			Enabled:         true,
			Detector:        DetectorProjectZomboidRCON,
			RCONAddress:     "127.0.0.1:27015",
			RCONPasswordEnv: "TEST_PROJECT_ZOMBOID_RCON_PASSWORD",
		},
	)
	if err != nil {
		t.Fatalf("PlayerCount retornou erro: %v", err)
	}
	if !called {
		t.Fatal("a função RCON não foi chamada")
	}
	if count != 2 {
		t.Fatalf("contagem inesperada: %d", count)
	}
}

func TestDetectorRegistryUsesFallbackToConfirmZero(t *testing.T) {
	primary := &fakePlayerDetector{
		detectorType: DetectorAMPPlayers,
		playerCount:  0,
	}
	fallback := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
		playerCount:  2,
	}

	registry, err := NewDetectorRegistry(primary, fallback)
	if err != nil {
		t.Fatalf("não foi possível criar o registro: %v", err)
	}

	count, err := registry.PlayerCount(context.Background(), Server{
		Instance:         "Palworld01",
		Enabled:          true,
		Detector:         DetectorAMPPlayers,
		FallbackDetector: DetectorPalworldRCON,
	})
	if err != nil {
		t.Fatalf("PlayerCount retornou erro: %v", err)
	}

	if count != 2 {
		t.Fatalf("contagem inesperada: %d", count)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf(
			"chamadas inesperadas: primário=%d fallback=%d",
			primary.calls,
			fallback.calls,
		)
	}
}

func TestDetectorRegistryUsesFallbackWhenPrimaryFails(t *testing.T) {
	primary := &fakePlayerDetector{
		detectorType: DetectorAMPPlayers,
		err:          errors.New("AMP indisponível"),
	}
	fallback := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
		playerCount:  1,
	}

	registry, err := NewDetectorRegistry(primary, fallback)
	if err != nil {
		t.Fatalf("não foi possível criar o registro: %v", err)
	}

	count, err := registry.PlayerCount(context.Background(), Server{
		Instance:         "Palworld01",
		Enabled:          true,
		Detector:         DetectorAMPPlayers,
		FallbackDetector: DetectorPalworldRCON,
	})
	if err != nil {
		t.Fatalf("PlayerCount retornou erro: %v", err)
	}

	if count != 1 {
		t.Fatalf("contagem inesperada: %d", count)
	}
}

func TestDetectorRegistrySkipsFallbackWhenPrimaryHasPlayers(t *testing.T) {
	primary := &fakePlayerDetector{
		detectorType: DetectorAMPPlayers,
		playerCount:  3,
	}
	fallback := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
		playerCount:  0,
	}

	registry, err := NewDetectorRegistry(primary, fallback)
	if err != nil {
		t.Fatalf("não foi possível criar o registro: %v", err)
	}

	count, err := registry.PlayerCount(context.Background(), Server{
		Instance:         "Palworld01",
		Enabled:          true,
		Detector:         DetectorAMPPlayers,
		FallbackDetector: DetectorPalworldRCON,
	})
	if err != nil {
		t.Fatalf("PlayerCount retornou erro: %v", err)
	}

	if count != 3 {
		t.Fatalf("contagem inesperada: %d", count)
	}
	if fallback.calls != 0 {
		t.Fatalf("o fallback não deveria ser consultado; chamadas=%d", fallback.calls)
	}
}

func TestDetectorRegistryRejectsNilDetector(
	t *testing.T,
) {
	t.Parallel()

	_, err := NewDetectorRegistry(
		nil,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para detector nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"é nulo",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDetectorRegistryRejectsUnknownDetector(
	t *testing.T,
) {
	t.Parallel()

	detector := &fakePlayerDetector{
		detectorType: Detector(
			"detector_desconhecido",
		),
	}

	_, err := NewDetectorRegistry(
		detector,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para detector desconhecido",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não é reconhecido",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDetectorRegistryRejectsDuplicateDetector(
	t *testing.T,
) {
	t.Parallel()

	first := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
	}

	second := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
	}

	_, err := NewDetectorRegistry(
		first,
		second,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para detector duplicado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"registrado mais de uma vez",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDetectorRegistryRoutesPlayerCount(
	t *testing.T,
) {
	t.Parallel()

	detector := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
		playerCount:  3,
	}

	registry, err := NewDetectorRegistry(
		detector,
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro: %v",
			err,
		)
	}

	server := Server{
		Instance: "ServidorTeste01",
		Enabled:  true,
		Detector: DetectorPalworldRCON,
	}

	playerCount, err := registry.PlayerCount(
		context.Background(),
		server,
	)
	if err != nil {
		t.Fatalf(
			"PlayerCount retornou erro: %v",
			err,
		)
	}

	if playerCount != 3 {
		t.Fatalf(
			"quantidade inesperada de jogadores: %d",
			playerCount,
		)
	}

	if detector.calls != 1 {
		t.Fatalf(
			"quantidade inesperada de chamadas ao detector: %d",
			detector.calls,
		)
	}
}

func TestDetectorRegistryRejectsDisabledServer(
	t *testing.T,
) {
	t.Parallel()

	detector := &fakePlayerDetector{
		detectorType: DetectorPalworldRCON,
	}

	registry, err := NewDetectorRegistry(
		detector,
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro: %v",
			err,
		)
	}

	_, err = registry.PlayerCount(
		context.Background(),
		Server{
			Instance: "ServidorDesabilitado01",
			Enabled:  false,
			Detector: DetectorPalworldRCON,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para servidor desabilitado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"está desabilitado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDetectorRegistryRejectsMissingDetector(
	t *testing.T,
) {
	t.Parallel()

	registry, err := NewDetectorRegistry(
		&fakePlayerDetector{
			detectorType: DetectorPalworldRCON,
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro: %v",
			err,
		)
	}

	_, err = registry.PlayerCount(
		context.Background(),
		Server{
			Instance: "Valheim01",
			Enabled:  true,
			Detector: DetectorSourceQuery,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para detector não registrado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não está registrado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestDetectorRegistryRejectsNegativePlayerCount(
	t *testing.T,
) {
	t.Parallel()

	registry, err := NewDetectorRegistry(
		&fakePlayerDetector{
			detectorType: DetectorPalworldRCON,
			playerCount:  -1,
		},
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro: %v",
			err,
		)
	}

	_, err = registry.PlayerCount(
		context.Background(),
		Server{
			Instance: "ServidorTeste01",
			Enabled:  true,
			Detector: DetectorPalworldRCON,
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para quantidade negativa",
		)
	}

	if !strings.Contains(
		err.Error(),
		"quantidade negativa",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestPalworldRCONDetectorUsesEnvironmentVariable(
	t *testing.T,
) {
	t.Setenv(
		"TEST_PALWORLD_RCON_PASSWORD",
		"senha-palworld",
	)

	var receivedAddress string
	var receivedPassword string

	detector := PalworldRCONDetector{
		playerCount: func(
			_ context.Context,
			address string,
			password string,
		) (int, error) {
			receivedAddress = address
			receivedPassword = password

			return 2, nil
		},
	}

	server := Server{
		Instance:        "AlamamaPal01",
		Enabled:         true,
		Detector:        DetectorPalworldRCON,
		RCONAddress:     "127.0.0.1:25575",
		RCONPasswordEnv: "TEST_PALWORLD_RCON_PASSWORD",
	}

	playerCount, err := detector.PlayerCount(
		context.Background(),
		server,
	)
	if err != nil {
		t.Fatalf(
			"PlayerCount retornou erro: %v",
			err,
		)
	}

	if playerCount != 2 {
		t.Fatalf(
			"quantidade inesperada de jogadores: %d",
			playerCount,
		)
	}

	if receivedAddress != "127.0.0.1:25575" {
		t.Fatalf(
			"endereço recebido pelo detector: %q",
			receivedAddress,
		)
	}

	if receivedPassword != "senha-palworld" {
		t.Fatalf(
			"senha recebida pelo detector: %q",
			receivedPassword,
		)
	}
}

func TestPalworldRCONDetectorRejectsMissingAddress(
	t *testing.T,
) {
	t.Parallel()

	detector := NewPalworldRCONDetector()

	_, err := detector.PlayerCount(
		context.Background(),
		Server{
			Instance:        "AlamamaPal01",
			Enabled:         true,
			Detector:        DetectorPalworldRCON,
			RCONPasswordEnv: "TEST_PASSWORD",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para endereço RCON ausente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"endereço RCON",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestPalworldRCONDetectorRejectsMissingPassword(
	t *testing.T,
) {
	t.Setenv(
		"TEST_MISSING_RCON_PASSWORD",
		"",
	)

	detector := NewPalworldRCONDetector()

	_, err := detector.PlayerCount(
		context.Background(),
		Server{
			Instance:        "AlamamaPal01",
			Enabled:         true,
			Detector:        DetectorPalworldRCON,
			RCONAddress:     "127.0.0.1:25575",
			RCONPasswordEnv: "TEST_MISSING_RCON_PASSWORD",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para senha ausente",
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

func TestPalworldRCONDetectorWrapsRCONError(
	t *testing.T,
) {
	t.Setenv(
		"TEST_RCON_ERROR_PASSWORD",
		"senha",
	)

	expectedErr := errors.New(
		"falha simulada",
	)

	detector := PalworldRCONDetector{
		playerCount: func(
			context.Context,
			string,
			string,
		) (int, error) {
			return 0, expectedErr
		},
	}

	_, err := detector.PlayerCount(
		context.Background(),
		Server{
			Instance:        "KalagaPal01",
			Enabled:         true,
			Detector:        DetectorPalworldRCON,
			RCONAddress:     "127.0.0.1:25576",
			RCONPasswordEnv: "TEST_RCON_ERROR_PASSWORD",
		},
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro RCON",
		)
	}

	if !strings.Contains(
		err.Error(),
		"falha simulada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
