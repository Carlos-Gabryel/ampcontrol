package amp

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	defaultApplicationRetryInterval = 3 * time.Second
	applicationAttemptTimeout       = 20 * time.Second
)

// StartApplicationUntilReady tenta iniciar a aplicação repetidamente.
//
// Esta função é usada principalmente quando a instância AMP estava
// completamente desligada. Depois de iniciar a instância, a API pode
// levar alguns segundos para ficar disponível.
//
// A função encerra quando:
//   - Core.Start for executado com sucesso;
//   - o contexto for cancelado;
//   - o prazo do contexto for excedido.
func (c *APIClient) StartApplicationUntilReady(
	ctx context.Context,
	baseURL string,
	retryInterval time.Duration,
) error {
	baseURL = strings.TrimSpace(baseURL)

	if baseURL == "" {
		return fmt.Errorf(
			"a URL da instância AMP não foi informada",
		)
	}

	if retryInterval <= 0 {
		retryInterval = defaultApplicationRetryInterval
	}

	var lastErr error

	for {
		if err := ctx.Err(); err != nil {
			return buildApplicationRetryError(
				baseURL,
				lastErr,
				err,
			)
		}

		attemptCtx, attemptCancel := context.WithTimeout(
			ctx,
			applicationAttemptTimeout,
		)

		err := c.StartApplication(
			attemptCtx,
			baseURL,
		)

		attemptCancel()

		if err == nil {
			return nil
		}

		lastErr = err

		timer := time.NewTimer(
			retryInterval,
		)

		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			return buildApplicationRetryError(
				baseURL,
				lastErr,
				ctx.Err(),
			)

		case <-timer.C:
		}
	}
}

// RestartApplication reinicia somente o processo do jogo.
//
// A instância AMP permanece ligada durante a operação.
//
// O processo é:
//   - executar Core.Stop;
//   - aguardar a aplicação aceitar Core.Start;
//   - executar Core.Start novamente.
func (c *APIClient) RestartApplication(
	ctx context.Context,
	baseURL string,
) error {
	baseURL = strings.TrimSpace(baseURL)

	if baseURL == "" {
		return fmt.Errorf(
			"a URL da instância AMP não foi informada",
		)
	}

	if err := c.StopApplication(
		ctx,
		baseURL,
	); err != nil {
		return fmt.Errorf(
			"não foi possível parar a aplicação antes do reinício: %w",
			err,
		)
	}

	if err := c.StartApplicationUntilReady(
		ctx,
		baseURL,
		defaultApplicationRetryInterval,
	); err != nil {
		return fmt.Errorf(
			"a aplicação foi parada, mas não pôde ser iniciada novamente: %w",
			err,
		)
	}

	return nil
}

func buildApplicationRetryError(
	baseURL string,
	lastErr error,
	contextErr error,
) error {
	if lastErr == nil {
		return fmt.Errorf(
			"a aplicação em %s não pôde ser iniciada: %w",
			baseURL,
			contextErr,
		)
	}

	return fmt.Errorf(
		"a aplicação em %s não pôde ser iniciada antes do prazo; "+
			"último erro: %v: %w",
		baseURL,
		lastErr,
		contextErr,
	)
}
