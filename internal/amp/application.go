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

// StartApplicationUntilReady inicia ou acorda a aplicação e aguarda
// até que Core.GetStatus confirme ApplicationStateReady.
//
// Esta função também é usada quando a instância AMP estava
// completamente desligada. Nesse cenário, depois de iniciar a
// instância, a API pode levar alguns segundos para ficar disponível.
//
// O processo possui duas fases:
//
//   - tentar Core.Start até que a API esteja disponível e aceite
//     a inicialização;
//   - depois que Core.Start for aceito, consultar Core.GetStatus
//     repetidamente até a aplicação chegar ao estado Ready.
//
// Depois que Core.Start é aceito, ele não é enviado novamente.
//
// A função encerra quando:
//
//   - Core.GetStatus retornar ApplicationStateReady;
//   - a aplicação entrar em Failed;
//   - a aplicação entrar em Suspended;
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

	startAccepted := false

	for {
		if err := ctx.Err(); err != nil {
			return buildApplicationRetryError(
				baseURL,
				lastErr,
				err,
			)
		}

		if !startAccepted {
			attemptCtx, attemptCancel := context.WithTimeout(
				ctx,
				applicationAttemptTimeout,
			)

			err := c.StartApplication(
				attemptCtx,
				baseURL,
			)

			attemptCancel()

			if err != nil {
				lastErr = fmt.Errorf(
					"Core.Start ainda não foi aceito: %w",
					err,
				)

				if err := waitApplicationRetry(
					ctx,
					retryInterval,
				); err != nil {
					return buildApplicationRetryError(
						baseURL,
						lastErr,
						err,
					)
				}

				continue
			}

			startAccepted = true
			lastErr = nil
		}

		statusCtx, statusCancel := context.WithTimeout(
			ctx,
			applicationAttemptTimeout,
		)

		status, err := c.GetApplicationStatus(
			statusCtx,
			baseURL,
		)

		statusCancel()

		if err != nil {
			lastErr = fmt.Errorf(
				"Core.GetStatus ainda não pôde ser consultado: %w",
				err,
			)
		} else {
			switch status.Phase() {
			case ApplicationPhaseOnline:
				return nil

			case ApplicationPhaseFailed:
				return fmt.Errorf(
					"a aplicação em %s entrou em estado de falha antes de ficar pronta: %s",
					baseURL,
					status.State.String(),
				)

			case ApplicationPhaseSuspended:
				return fmt.Errorf(
					"a aplicação em %s ficou suspensa antes de ficar pronta: %s",
					baseURL,
					status.State.String(),
				)

			default:
				lastErr = fmt.Errorf(
					"a aplicação ainda não está pronta; estado atual: %s",
					status.State.String(),
				)
			}
		}

		if err := waitApplicationRetry(
			ctx,
			retryInterval,
		); err != nil {
			return buildApplicationRetryError(
				baseURL,
				lastErr,
				err,
			)
		}
	}
}

// RestartApplication reinicia somente o processo do jogo.
//
// A instância AMP permanece ligada durante a operação.
//
// O processo é:
//
//   - executar Core.Stop;
//   - aguardar a aplicação aceitar Core.Start;
//   - executar Core.Start novamente;
//   - aguardar Core.GetStatus confirmar Ready.
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

func waitApplicationRetry(
	ctx context.Context,
	retryInterval time.Duration,
) error {
	timer := time.NewTimer(
		retryInterval,
	)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

func buildApplicationRetryError(
	baseURL string,
	lastErr error,
	contextErr error,
) error {
	if lastErr == nil {
		return fmt.Errorf(
			"a aplicação em %s não ficou pronta: %w",
			baseURL,
			contextErr,
		)
	}

	return fmt.Errorf(
		"a aplicação em %s não ficou pronta antes do prazo; "+
			"último erro: %v: %w",
		baseURL,
		lastErr,
		contextErr,
	)
}
