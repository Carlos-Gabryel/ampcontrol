package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/idle"
	"github.com/rs/zerolog"
)

type recordingIdleNotificationTarget struct {
	messages       []string
	err            error
	auditCalls     int
	auditServer    idle.Server
	auditStartedAt time.Time
	auditErr       error
}

func (n *recordingIdleNotificationTarget) RecordAutomaticIdle(
	server idle.Server,
	startedAt time.Time,
	operationErr error,
) {
	n.auditCalls++
	n.auditServer = server
	n.auditStartedAt = startedAt
	n.auditErr = operationErr
}

func (n *recordingIdleNotificationTarget) SendNotification(
	content string,
) error {
	n.messages = append(
		n.messages,
		content,
	)

	return n.err
}

type recordingIdleNotificationNextStopper struct {
	calls int
	err   error
}

func (s *recordingIdleNotificationNextStopper) StopApplication(
	context.Context,
	idle.Server,
) error {
	s.calls++

	return s.err
}

func TestIdleNotificationStopperSendsWarningAndSuccess(
	t *testing.T,
) {
	t.Parallel()

	next := &recordingIdleNotificationNextStopper{}
	notifier := &recordingIdleNotificationTarget{}

	stopper, err := newIdleNotificationStopper(
		next,
		notifier,
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf(
			"newIdleNotificationStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerModeActive,
			IdleTimeout: 15 * time.Minute,
		},
	)
	if err != nil {
		t.Fatalf(
			"StopApplication retornou erro: %v",
			err,
		)
	}

	if next.calls != 1 {
		t.Fatalf(
			"o stopper real deveria ser chamado uma vez; chamadas=%d",
			next.calls,
		)
	}

	if len(notifier.messages) != 2 {
		t.Fatalf(
			"eram esperadas duas notificações; encontradas=%d",
			len(notifier.messages),
		)
	}

	if notifier.auditCalls != 1 ||
		notifier.auditServer.Instance != "AlamamaPal01" ||
		notifier.auditStartedAt.IsZero() ||
		notifier.auditErr != nil {
		t.Fatalf("auditoria de sucesso inesperada: %#v", notifier)
	}

	if !strings.Contains(
		notifier.messages[0],
		"Alamama",
	) ||
		!strings.Contains(
			notifier.messages[0],
			"15 minutos",
		) {
		t.Fatalf(
			"aviso inesperado: %q",
			notifier.messages[0],
		)
	}

	if !strings.Contains(
		notifier.messages[1],
		"entrou em modo Idle",
	) ||
		!strings.Contains(
			notifier.messages[1],
			"/amp iniciar",
		) {
		t.Fatalf(
			"confirmação inesperada: %q",
			notifier.messages[1],
		)
	}
}

func TestIdleNotificationStopperSendsFailure(
	t *testing.T,
) {
	t.Parallel()

	expectedErr := errors.New(
		"falha de teste no Core.Stop",
	)

	next := &recordingIdleNotificationNextStopper{
		err: expectedErr,
	}

	notifier := &recordingIdleNotificationTarget{}

	stopper, err := newIdleNotificationStopper(
		next,
		notifier,
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf(
			"newIdleNotificationStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "KalagaPal01",
			DisplayName: "Kalaga",
			Enabled:     true,
			Mode:        idle.ServerModeActive,
			IdleTimeout: 15 * time.Minute,
		},
	)

	if !errors.Is(
		err,
		expectedErr,
	) {
		t.Fatalf(
			"o erro real deveria ser preservado; erro=%v",
			err,
		)
	}

	if next.calls != 1 {
		t.Fatalf(
			"o stopper real deveria ser chamado uma vez; chamadas=%d",
			next.calls,
		)
	}

	if len(notifier.messages) != 2 {
		t.Fatalf(
			"eram esperadas duas notificações; encontradas=%d",
			len(notifier.messages),
		)
	}

	if notifier.auditCalls != 1 ||
		notifier.auditServer.Instance != "KalagaPal01" ||
		notifier.auditStartedAt.IsZero() ||
		!errors.Is(notifier.auditErr, expectedErr) {
		t.Fatalf("auditoria de falha inesperada: %#v", notifier)
	}

	if !strings.Contains(
		notifier.messages[0],
		"Kalaga",
	) {
		t.Fatalf(
			"aviso inesperado: %q",
			notifier.messages[0],
		)
	}

	if !strings.Contains(
		notifier.messages[1],
		"Não foi possível colocar",
	) ||
		!strings.Contains(
			notifier.messages[1],
			"Kalaga",
		) {
		t.Fatalf(
			"falha inesperada: %q",
			notifier.messages[1],
		)
	}
}

func TestIdleNotificationFailureDoesNotBlockStop(
	t *testing.T,
) {
	t.Parallel()

	next := &recordingIdleNotificationNextStopper{}

	notifier := &recordingIdleNotificationTarget{
		err: errors.New(
			"Discord indisponível",
		),
	}

	stopper, err := newIdleNotificationStopper(
		next,
		notifier,
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf(
			"newIdleNotificationStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerModeActive,
			IdleTimeout: 15 * time.Minute,
		},
	)
	if err != nil {
		t.Fatalf(
			"falha de notificação não deveria impedir a parada: %v",
			err,
		)
	}

	if next.calls != 1 {
		t.Fatalf(
			"o stopper real deveria continuar sendo chamado; chamadas=%d",
			next.calls,
		)
	}

	if len(notifier.messages) != 2 {
		t.Fatalf(
			"as duas notificações deveriam ter sido tentadas; encontradas=%d",
			len(notifier.messages),
		)
	}
}

func TestObserveModeDoesNotReachNotificationStopper(
	t *testing.T,
) {
	t.Parallel()

	next := &recordingIdleNotificationNextStopper{}
	notifier := &recordingIdleNotificationTarget{}

	notificationStopper, err := newIdleNotificationStopper(
		next,
		notifier,
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf(
			"newIdleNotificationStopper retornou erro: %v",
			err,
		)
	}

	stopper, err := newModeAwareStopper(
		notificationStopper,
	)
	if err != nil {
		t.Fatalf(
			"newModeAwareStopper retornou erro: %v",
			err,
		)
	}

	err = stopper.StopApplication(
		context.Background(),
		idle.Server{
			Instance:    "AlamamaPal01",
			DisplayName: "Alamama",
			Enabled:     true,
			Mode:        idle.ServerModeObserve,
			IdleTimeout: 15 * time.Minute,
		},
	)

	if !errors.Is(
		err,
		errIdleObservationOnly,
	) {
		t.Fatalf(
			"modo observe deveria permanecer bloqueado; erro=%v",
			err,
		)
	}

	if next.calls != 0 {
		t.Fatalf(
			"o stopper real não poderia ser chamado; chamadas=%d",
			next.calls,
		)
	}

	if len(notifier.messages) != 0 {
		t.Fatalf(
			"observe não poderia gerar notificações de parada; encontradas=%d",
			len(notifier.messages),
		)
	}
}

func TestNewIdleNotificationStopperRejectsNilNext(
	t *testing.T,
) {
	t.Parallel()

	notifier := &recordingIdleNotificationTarget{}

	_, err := newIdleNotificationStopper(
		nil,
		notifier,
		zerolog.Nop(),
	)

	if err == nil {
		t.Fatal(
			"stopper real nulo deveria retornar erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"parada",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestNewIdleNotificationStopperRejectsNilNotifier(
	t *testing.T,
) {
	t.Parallel()

	next := &recordingIdleNotificationNextStopper{}

	_, err := newIdleNotificationStopper(
		next,
		nil,
		zerolog.Nop(),
	)

	if err == nil {
		t.Fatal(
			"notificante nulo deveria retornar erro",
		)
	}

	if !strings.Contains(
		err.Error(),
		"notificante",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
