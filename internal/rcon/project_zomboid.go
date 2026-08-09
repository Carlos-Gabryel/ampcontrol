package rcon

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var projectZomboidPlayersPattern = regexp.MustCompile(
	`(?i)players\s+connected\s*\(\s*(\d+)\s*\)`,
)

// ProjectZomboidPlayerCount consulta o comando players do servidor
// Project Zomboid pelo protocolo Source RCON.
func ProjectZomboidPlayerCount(
	ctx context.Context,
	address string,
	password string,
) (int, error) {
	output, err := ExecuteCommand(
		ctx,
		address,
		password,
		"players",
	)
	if err != nil {
		return 0, err
	}

	return parseProjectZomboidPlayerCount(output)
}

func parseProjectZomboidPlayerCount(
	output string,
) (int, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0, fmt.Errorf(
			"o comando players retornou uma resposta vazia",
		)
	}

	match := projectZomboidPlayersPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, fmt.Errorf(
			"não foi possível interpretar a quantidade retornada pelo comando players",
		)
	}

	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf(
			"a quantidade retornada pelo comando players é inválida: %w",
			err,
		)
	}

	return count, nil
}
