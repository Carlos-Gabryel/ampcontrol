package app

import (
	"context"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/idle"
)

type dashboardFakePlayerDetector struct {
	detectorType idle.Detector
	playerCount  int
}

func (d dashboardFakePlayerDetector) DetectorType() idle.Detector {
	return d.detectorType
}

func (d dashboardFakePlayerDetector) PlayerCount(
	context.Context,
	idle.Server,
) (int, error) {
	return d.playerCount, nil
}

func TestDashboardPlayerCountResolverUsesConfiguredFallback(t *testing.T) {
	t.Parallel()

	detectors, err := idle.NewDetectorRegistry(
		dashboardFakePlayerDetector{
			detectorType: idle.DetectorAMPPlayers,
			playerCount:  0,
		},
		dashboardFakePlayerDetector{
			detectorType: idle.DetectorProjectZomboidRCON,
			playerCount:  1,
		},
	)
	if err != nil {
		t.Fatalf("não foi possível criar os detectores: %v", err)
	}

	resolver := dashboardPlayerCountResolver{
		config: idle.Config{Servers: []idle.Server{
			{
				Instance:         "TheWalkingRats01",
				Enabled:          true,
				Detector:         idle.DetectorAMPPlayers,
				FallbackDetector: idle.DetectorProjectZomboidRCON,
			},
		}},
		detectors: detectors,
	}

	count, applies, err := resolver.ResolvePlayerCount(
		context.Background(),
		"TheWalkingRats01",
	)
	if err != nil {
		t.Fatalf("ResolvePlayerCount retornou erro: %v", err)
	}
	if !applies {
		t.Fatal("o fallback configurado deveria ser aplicado")
	}
	if count != 1 {
		t.Fatalf("contagem inesperada: %d", count)
	}
}

func TestDashboardPlayerCountResolverSkipsServerWithoutFallback(t *testing.T) {
	t.Parallel()

	resolver := dashboardPlayerCountResolver{
		config: idle.Config{Servers: []idle.Server{
			{
				Instance: "Valheim01",
				Enabled:  true,
				Detector: idle.DetectorAMPPlayers,
			},
		}},
		detectors: &idle.DetectorRegistry{},
	}

	_, applies, err := resolver.ResolvePlayerCount(
		context.Background(),
		"Valheim01",
	)
	if err != nil {
		t.Fatalf("ResolvePlayerCount retornou erro: %v", err)
	}
	if applies {
		t.Fatal("um servidor sem fallback não deveria ser alterado")
	}
}

func TestDashboardPlayerCountResolverReplaceConfig(t *testing.T) {
	detectors, err := idle.NewDetectorRegistry(
		dashboardFakePlayerDetector{detectorType: idle.DetectorAMPPlayers, playerCount: 0},
		dashboardFakePlayerDetector{detectorType: idle.DetectorPalworldRCON, playerCount: 3},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := dashboardPlayerCountResolver{
		config:    idle.Config{Servers: []idle.Server{{Instance: "KalagaPal01", Enabled: true, Detector: idle.DetectorAMPPlayers}}},
		detectors: detectors,
	}
	resolver.ReplaceConfig(idle.Config{Servers: []idle.Server{{
		Instance: "KalagaPal01", Enabled: true, Detector: idle.DetectorAMPPlayers, FallbackDetector: idle.DetectorPalworldRCON,
	}}})
	count, applies, err := resolver.ResolvePlayerCount(context.Background(), "KalagaPal01")
	if err != nil || !applies || count != 3 {
		t.Fatalf("configuração substituída não foi usada: count=%d applies=%v err=%v", count, applies, err)
	}
}
