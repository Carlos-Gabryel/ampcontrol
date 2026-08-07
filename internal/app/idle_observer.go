package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/idle"
	"github.com/alabamaamp/palcontrol/internal/operation"
	"github.com/rs/zerolog"
)

const idleObserverConfigPath = "config/idle.json"

var errIdleObservationOnly = errors.New(
	"modo de observação: Core.Stop não foi executado",
)

// idleObserver executa o novo motor genérico sem permitir
// que ele pare aplicações reais.
//
// O monitor antigo permanece responsável pelas paradas enquanto
// comparamos o comportamento das duas implementações.
type idleObserver struct {
	engine *idle.Engine
	config idle.Config
	log    zerolog.Logger
}

// observationOnlyStopper implementa idle.ApplicationStopper,
// mas nunca envia Core.Stop ao AMP.
type observationOnlyStopper struct{}

func (
	observationOnlyStopper,
) StopApplication(
	context.Context,
	idle.Server,
) error {
	return errIdleObservationOnly
}

func newIdleObserver(
	ampClient *amp.APIClient,
	operationManager *operation.Manager,
	log zerolog.Logger,
) (*idleObserver, error) {
	if ampClient == nil {
		return nil, fmt.Errorf(
			"o cliente AMP do observador genérico de Idle não foi informado",
		)
	}

	if operationManager == nil {
		return nil, fmt.Errorf(
			"o gerenciador de operações do observador genérico de Idle não foi informado",
		)
	}

	idleConfig, err := idle.Load(
		idleObserverConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível carregar a configuração do observador genérico de Idle: %w",
			err,
		)
	}

	detectors, err := idle.NewDefaultDetectorRegistry()
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o registro de detectores do observador de Idle: %w",
			err,
		)
	}

	ampAdapter, err := idle.NewAMPAdapter(
		ampClient,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o adaptador AMP do observador de Idle: %w",
			err,
		)
	}

	protectedStopper, err := idle.NewLockedStopper(
		operationManager,
		observationOnlyStopper{},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível proteger as paradas automáticas do observador de Idle: %w",
			err,
		)
	}

	engine, err := idle.NewEngine(
		idleConfig,
		detectors,
		ampAdapter,
		protectedStopper,
		buildIdleObservationEventHandler(
			log,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o motor genérico de Idle em observação: %w",
			err,
		)
	}

	return &idleObserver{
		engine: engine,
		config: idleConfig,
		log:    log,
	}, nil
}

func (o *idleObserver) Run(
	ctx context.Context,
) {
	if o == nil ||
		o.engine == nil ||
		ctx == nil {
		return
	}

	enabledServers := o.config.EnabledServers()

	o.log.Info().
		Str("idle_mode", "observation").
		Str("config_path", idleObserverConfigPath).
		Int("registered_servers", len(o.config.Servers)).
		Int("enabled_servers", len(enabledServers)).
		Dur("check_interval", o.config.CheckInterval).
		Msg("Observador genérico de Idle iniciado")

	o.engine.Run(
		ctx,
	)

	o.log.Info().
		Str("idle_mode", "observation").
		Msg("Observador genérico de Idle encerrado")
}

func buildIdleObservationEventHandler(
	log zerolog.Logger,
) idle.EventHandler {
	return func(
		_ context.Context,
		event idle.Event,
	) {
		eventLog := log.With().
			Str("idle_mode", "observation").
			Str("server", event.Instance).
			Str("display_name", event.DisplayName).
			Str(
				"runtime_state",
				string(event.RuntimeState),
			).
			Logger()

		switch event.Type {
		case idle.EventRuntimeUnavailable:
			eventLog.Warn().
				Err(event.Err).
				Msg("Observador não conseguiu consultar o estado da aplicação")

		case idle.EventRuntimeNotOnline:
			eventLog.Debug().
				Msg("Observador ignorou aplicação que não está Online")

		case idle.EventStartupGraceStarted:
			eventLog.Info().
				Dur(
					"grace_remaining",
					event.GraceRemaining,
				).
				Msg("Observador iniciou a proteção após a aplicação ficar Online")

		case idle.EventStartupGraceActive:
			eventLog.Debug().
				Dur(
					"grace_elapsed",
					event.GraceElapsed,
				).
				Dur(
					"grace_remaining",
					event.GraceRemaining,
				).
				Msg("Observador mantém a proteção inicial ativa")

		case idle.EventDetectorFailed:
			eventLog.Warn().
				Err(event.Err).
				Msg("Observador não conseguiu consultar os jogadores")

		case idle.EventPlayersPresent:
			eventLog.Debug().
				Int(
					"players",
					event.PlayerCount,
				).
				Msg("Observador encontrou jogadores conectados")

		case idle.EventIdleTimerStarted:
			eventLog.Info().
				Dur(
					"idle_remaining",
					event.IdleRemaining,
				).
				Msg("Observador iniciou o contador individual de inatividade")

		case idle.EventIdleTimerActive:
			eventLog.Debug().
				Dur(
					"idle_elapsed",
					event.IdleElapsed,
				).
				Dur(
					"idle_remaining",
					event.IdleRemaining,
				).
				Msg("Observador mantém o contador de inatividade ativo")

		case idle.EventFinalCheckFailed:
			eventLog.Warn().
				Err(event.Err).
				Dur(
					"idle_elapsed",
					event.IdleElapsed,
				).
				Msg("Observador cancelou a decisão por falha na confirmação final")

		case idle.EventStopCancelledPlayers:
			eventLog.Info().
				Int(
					"players",
					event.PlayerCount,
				).
				Msg("Observador cancelou a parada porque jogadores entraram")

		case idle.EventStopCancelledState:
			eventLog.Info().
				Msg("Observador cancelou a parada porque o estado da aplicação mudou")

		case idle.EventStopStarting:
			eventLog.Info().
				Dur(
					"idle_elapsed",
					event.IdleElapsed,
				).
				Msg("Modo observação: o limite foi atingido e uma parada automática seria tentada")

		case idle.EventStopFailed:
			var busyError *idle.OperationBusyError

			if errors.As(
				event.Err,
				&busyError,
			) {
				activeOperation := strings.TrimSpace(
					busyError.Active.Operation,
				)

				if activeOperation == "" {
					activeOperation = "operação não identificada"
				}

				eventLog.Info().
					Str(
						"active_operation",
						activeOperation,
					).
					Msg("Modo observação: parada automática adiada porque a instância está ocupada")

				return
			}

			if errors.Is(
				event.Err,
				errIdleObservationOnly,
			) {
				eventLog.Debug().
					Msg("Modo observação: Core.Stop foi intencionalmente bloqueado")

				return
			}

			eventLog.Error().
				Err(event.Err).
				Msg("Observador encontrou uma falha inesperada na simulação da parada")

		case idle.EventStopSucceeded:
			eventLog.Warn().
				Msg("Observador registrou uma parada concluída inesperadamente")

		default:
			eventLog.Debug().
				Str(
					"event_type",
					string(event.Type),
				).
				Msg("Evento do observador genérico de Idle")
		}
	}
}
