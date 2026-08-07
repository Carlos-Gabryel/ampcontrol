package amp

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartApplicationUntilReadyRejectsEmptyURL(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	err := client.StartApplicationUntilReady(
		context.Background(),
		"   ",
		time.Millisecond,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para URL vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"URL da instância AMP não foi informada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestRestartApplicationRejectsEmptyURL(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	err := client.RestartApplication(
		context.Background(),
		"",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para URL vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"URL da instância AMP não foi informada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestStartApplicationUntilReadyHonorsCanceledContext(
	t *testing.T,
) {
	t.Parallel()

	client := NewAPIClient(
		"usuario",
		"senha",
	)

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	cancel()

	err := client.StartApplicationUntilReady(
		ctx,
		"http://127.0.0.1:65535",
		time.Millisecond,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para contexto cancelado",
		)
	}

	if !strings.Contains(
		err.Error(),
		"context canceled",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestBuildApplicationRetryErrorIncludesLastError(
	t *testing.T,
) {
	t.Parallel()

	lastErr := context.DeadlineExceeded
	contextErr := context.Canceled

	err := buildApplicationRetryError(
		"http://127.0.0.1:8080",
		lastErr,
		contextErr,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"último erro",
	) {
		t.Fatalf(
			"o erro não contém o último erro registrado: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"context canceled",
	) {
		t.Fatalf(
			"o erro não contém o cancelamento do contexto: %v",
			err,
		)
	}
}
