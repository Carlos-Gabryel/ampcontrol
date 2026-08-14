package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/idle"
)

type dashboardPlayerCountResolver struct {
	configMu  sync.RWMutex
	config    idle.Config
	detectors *idle.DetectorRegistry
}

func (r *dashboardPlayerCountResolver) ReplaceConfig(config idle.Config) {
	if r == nil {
		return
	}
	r.configMu.Lock()
	r.config = idle.Config{
		CheckInterval: config.CheckInterval,
		Servers:       append([]idle.Server(nil), config.Servers...),
	}
	r.configMu.Unlock()
}

func (r *dashboardPlayerCountResolver) configSnapshot() idle.Config {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return idle.Config{
		CheckInterval: r.config.CheckInterval,
		Servers:       append([]idle.Server(nil), r.config.Servers...),
	}
}

func newDashboardPlayerCountResolver(
	ampClient *amp.APIClient,
	inventory amp.InstanceDiscoverer,
	idleConfig idle.Config,
) (*dashboardPlayerCountResolver, error) {
	ampAdapter, err := idle.NewAMPAdapterWithInventory(ampClient, inventory)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o adaptador AMP do painel: %w",
			err,
		)
	}

	ampPlayersDetector, err := idle.NewAMPPlayersDetector(ampAdapter)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o detector AMP do painel: %w",
			err,
		)
	}

	detectors, err := idle.NewDefaultDetectorRegistry(ampPlayersDetector)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar os detectores do painel: %w",
			err,
		)
	}

	return &dashboardPlayerCountResolver{
		config:    idleConfig,
		detectors: detectors,
	}, nil
}

func (r *dashboardPlayerCountResolver) ResolvePlayerCount(
	ctx context.Context,
	instance string,
) (
	int,
	bool,
	error,
) {
	if r == nil || r.detectors == nil {
		return 0, false, fmt.Errorf(
			"o resolvedor de jogadores do painel não foi inicializado",
		)
	}

	server, exists := r.configSnapshot().FindServer(instance)
	if !exists || strings.TrimSpace(string(server.FallbackDetector)) == "" {
		return 0, false, nil
	}

	count, err := r.detectors.PlayerCount(ctx, server)
	if err != nil {
		return 0, true, err
	}

	return count, true, nil
}
