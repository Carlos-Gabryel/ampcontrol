package idle

import (
	"context"
	"fmt"
	"strings"

	"github.com/alabamaamp/ampcontrol/internal/rcon"
)

type rconPlayerCountFunc func(
	ctx context.Context,
	address string,
	password string,
) (int, error)

// PalworldRCONDetector consulta jogadores de um servidor Palworld
// executando o comando ShowPlayers pelo protocolo RCON.
type PalworldRCONDetector struct {
	playerCount rconPlayerCountFunc
}

// NewPalworldRCONDetector cria o detector real usado em produção.
func NewPalworldRCONDetector() PalworldRCONDetector {
	return PalworldRCONDetector{
		playerCount: rcon.PlayerCount,
	}
}

func (
	PalworldRCONDetector,
) DetectorType() Detector {
	return DetectorPalworldRCON
}

func (d PalworldRCONDetector) PlayerCount(
	ctx context.Context,
	server Server,
) (int, error) {
	if ctx == nil {
		return 0, fmt.Errorf(
			"o contexto da consulta RCON é nulo",
		)
	}

	address := strings.TrimSpace(
		server.RCONAddress,
	)

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
		playerCountFunction = rcon.PlayerCount
	}

	playerCount, err := playerCountFunction(
		ctx,
		address,
		password,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"consulta Palworld RCON em %s falhou: %w",
			address,
			err,
		)
	}

	if playerCount < 0 {
		return 0, fmt.Errorf(
			"a consulta Palworld RCON retornou uma quantidade negativa de jogadores: %d",
			playerCount,
		)
	}

	return playerCount, nil
}
