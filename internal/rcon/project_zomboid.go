package rcon

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
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
		return 0, errors.New(i18n.Choose("o comando players retornou uma resposta vazia", "the players command returned an empty response"))
	}

	match := projectZomboidPlayersPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return 0, errors.New(i18n.Choose("não foi possível interpretar a quantidade retornada pelo comando players", "could not parse the count returned by the players command"))
	}

	count, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf(
			i18n.Choose("a quantidade retornada pelo comando players é inválida: %w", "the count returned by the players command is invalid: %w"),
			err,
		)
	}

	return count, nil
}
