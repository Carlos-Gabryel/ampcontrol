package amp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

// ConsoleEntry é uma linha do console da instância devolvida por
// Core.GetUpdates. Campos desconhecidos são ignorados.
type ConsoleEntry struct {
	Timestamp string `json:"Timestamp"`
	Contents  string `json:"Contents"`
}

type consoleUpdatesResponse struct {
	ConsoleEntries []ConsoleEntry `json:"ConsoleEntries"`
}

var valheimJoinCodePattern = regexp.MustCompile(`(?i)join code (\d+)`)

// GetConsoleEntries abre uma sessão nova na instância e lê o console com
// Core.GetUpdates. Uma sessão nova recebe as últimas linhas como histórico.
func (c *APIClient) GetConsoleEntries(
	ctx context.Context,
	baseURL string,
) ([]ConsoleEntry, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, errors.New(i18n.Choose("a URL da instância AMP não foi informada", "the AMP instance URL was not provided"))
	}

	sessionID, err := c.login(ctx, baseURL)
	if err != nil {
		return nil, err
	}

	var response consoleUpdatesResponse

	err = c.postJSON(
		ctx,
		buildAPIURL(baseURL, "Core", "GetUpdates"),
		sessionID,
		authenticatedRequest{SessionID: sessionID},
		&response,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"falha chamando Core.GetUpdates: %w",
			err,
		)
	}

	return response.ConsoleEntries, nil
}

// ExtractValheimJoinCode devolve o código de convite da última linha do
// console que o contém, ou "" quando nenhuma linha o informa.
func ExtractValheimJoinCode(entries []ConsoleEntry) string {
	for index := len(entries) - 1; index >= 0; index-- {
		match := valheimJoinCodePattern.FindStringSubmatch(entries[index].Contents)
		if match != nil {
			return match[1]
		}
	}

	return ""
}
