package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/alabamaamp/ampcontrol/internal/idle"
)

type dashboardPlayerCountResolver struct {
	config    idle.Config
	detectors *idle.DetectorRegistry
}

func newDashboardPlayerCountResolver(
	ampClient *amp.APIClient,
	idleConfig idle.Config,
) (*dashboardPlayerCountResolver, error) {
	ampAdapter, err := idle.NewAMPAdapter(ampClient)
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

	server, exists := r.config.FindServer(instance)
	if !exists || strings.TrimSpace(string(server.FallbackDetector)) == "" {
		return 0, false, nil
	}

	count, err := r.detectors.PlayerCount(ctx, server)
	if err != nil {
		return 0, true, err
	}

	return count, true, nil
}
