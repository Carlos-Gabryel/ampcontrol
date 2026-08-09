package idle

import (
	"context"
	"fmt"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/rcon"
)

// ProjectZomboidRCONDetector consulta jogadores pelo comando players
// quando a métrica Active Users do AMP não representa as conexões reais.
type ProjectZomboidRCONDetector struct {
	playerCount rconPlayerCountFunc
}

// NewProjectZomboidRCONDetector cria o detector real usado em produção.
func NewProjectZomboidRCONDetector() ProjectZomboidRCONDetector {
	return ProjectZomboidRCONDetector{
		playerCount: rcon.ProjectZomboidPlayerCount,
	}
}

func (
	ProjectZomboidRCONDetector,
) DetectorType() Detector {
	return DetectorProjectZomboidRCON
}

func (d ProjectZomboidRCONDetector) PlayerCount(
	ctx context.Context,
	server Server,
) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf(
			"o contexto da consulta RCON é nulo",
		)
	}

	address := strings.TrimSpace(server.RCONAddress)
	if address == "" {
		return 0, fmt.Errorf(
			"o endereço RCON da instância %s não foi informado",
			server.Instance,
		)
	}

	password, err := server.RCONPassword()
	if err != nil {
		return 0, err
	}

	playerCountFunction := d.playerCount
	if playerCountFunction == nil {
		playerCountFunction = rcon.ProjectZomboidPlayerCount
	}

	playerCount, err := playerCountFunction(
		ctx,
		address,
		password,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"consulta Project Zomboid RCON em %s falhou: %w",
			address,
			err,
		)
	}

	if playerCount < 0 {
		return 0, fmt.Errorf(
			"a consulta Project Zomboid RCON retornou uma quantidade negativa de jogadores: %d",
			playerCount,
		)
	}

	return playerCount, nil
}
