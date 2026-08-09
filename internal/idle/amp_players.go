package idle

import (
	"context"
	"fmt"
)

type AMPPlayerCountProvider interface {
	PlayerCount(
		ctx context.Context,
		server Server,
	) (int, error)
}

type AMPPlayersDetector struct {
	provider AMPPlayerCountProvider
}

func NewAMPPlayersDetector(
	provider AMPPlayerCountProvider,
) (*AMPPlayersDetector, error) {
	if provider == nil {
		return nil, fmt.Errorf(
			"o provedor de jogadores da API AMP não foi informado",
		)
	}

	return &AMPPlayersDetector{provider: provider}, nil
}

func (*AMPPlayersDetector) DetectorType() Detector {
	return DetectorAMPPlayers
}

func (d *AMPPlayersDetector) PlayerCount(
	ctx context.Context,
	server Server,
) (int, error) {
	if d == nil || d.provider == nil {
		return 0, fmt.Errorf(
			"o detector de jogadores da API AMP não foi inicializado",
		)
	}

	return d.provider.PlayerCount(ctx, server)
}
