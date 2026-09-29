package idle

import (
	"context"
	"errors"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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
		return nil, errors.New(i18n.Choose("o provedor de jogadores da API AMP não foi informado", "the AMP API player provider was not provided"))
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
		return 0, errors.New(i18n.Choose("o detector de jogadores da API AMP não foi inicializado", "the AMP API player detector was not initialized"))
	}

	return d.provider.PlayerCount(ctx, server)
}
