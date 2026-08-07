package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alabamaamp/palcontrol/internal/idle"
	"github.com/rs/zerolog"
)

// idleNotifier representa somente a capacidade necessária pelo
// motor de Idle para publicar uma notificação operacional.
//
// O motor não precisa conhecer Discord, canais, tokens ou Disgo.
type idleNotifier interface {
	SendNotification(
		content string,
	) error
}

// idleNotificationStopper envolve somente a parada real.
//
// Ele é instalado dentro do LockedStopper. Dessa forma, as mensagens
// são enviadas apenas depois que a instância recebeu o bloqueio
// exclusivo necessário para a operação automática.
type idleNotificationStopper struct {
	next     idle.ApplicationStopper
	notifier idleNotifier
	log      zerolog.Logger
}

func newIdleNotificationStopper(
	next idle.ApplicationStopper,
	notifier idleNotifier,
	log zerolog.Logger,
) (*idleNotificationStopper, error) {
	if next == nil {
		return nil, fmt.Errorf(
			"o mecanismo real de parada do Idle não foi informado",
		)
	}

	if notifier == nil {
		return nil, fmt.Errorf(
			"o notificante do Idle não foi informado",
		)
	}

	return &idleNotificationStopper{
		next:     next,
		notifier: notifier,
		log:      log,
	}, nil
}

func (s *idleNotificationStopper) StopApplication(
	ctx context.Context,
	server idle.Server,
) error {
	if s == nil ||
		s.next == nil ||
		s.notifier == nil {
		return fmt.Errorf(
			"o mecanismo de parada com notificações não está disponível",
		)
	}

	displayName := idleNotificationDisplayName(
		server,
	)

	warningMessage := fmt.Sprintf(
		"⚠️ **%s está sem jogadores há %s.**\n"+
			"O processo do jogo será encerrado e o servidor entrará em modo Idle.",
		displayName,
		formatIdleNotificationDuration(
			server.IdleTimeout,
		),
	)

	if err := s.notifier.SendNotification(
		warningMessage,
	); err != nil {
		s.log.Warn().
			Err(err).
			Str("server", server.Instance).
			Msg("Não foi possível enviar o aviso de inatividade do motor de Idle")
	}

	err := s.next.StopApplication(
		ctx,
		server,
	)
	if err != nil {
		failureMessage := fmt.Sprintf(
			"❌ Não foi possível colocar **%s** em modo Idle automaticamente.\n"+
				"O erro foi registrado nos logs do serviço.",
			displayName,
		)

		if notificationErr := s.notifier.SendNotification(
			failureMessage,
		); notificationErr != nil {
			s.log.Warn().
				Err(notificationErr).
				Str("server", server.Instance).
				Msg("Não foi possível enviar a falha de parada do motor de Idle")
		}

		return err
	}

	successMessage := fmt.Sprintf(
		"💤 **%s entrou em modo Idle por inatividade.**\n"+
			"Use `/amp iniciar` quando quiser jogar novamente.",
		displayName,
	)

	if err := s.notifier.SendNotification(
		successMessage,
	); err != nil {
		s.log.Warn().
			Err(err).
			Str("server", server.Instance).
			Msg("Não foi possível enviar a confirmação de Idle do motor genérico")
	}

	return nil
}

func idleNotificationDisplayName(
	server idle.Server,
) string {
	displayName := strings.TrimSpace(
		server.DisplayName,
	)

	if displayName != "" {
		return displayName
	}

	instance := strings.TrimSpace(
		server.Instance,
	)

	if instance != "" {
		return instance
	}

	return "Servidor"
}

func formatIdleNotificationDuration(
	duration time.Duration,
) string {
	minutes := int(
		duration.Round(
			time.Minute,
		).Minutes(),
	)

	if minutes == 1 {
		return "1 minuto"
	}

	return fmt.Sprintf(
		"%d minutos",
		minutes,
	)
}
