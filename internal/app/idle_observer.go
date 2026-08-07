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

// idleObserver executa o motor genérico de Idle.
//
// A autoridade de parada é definida individualmente pela configuração
// de cada servidor:
//
//   - observe: o motor executa todas as verificações, mas Core.Stop
//     permanece bloqueado;
//   - active: o motor pode executar a parada real através do AMP,
//     sempre protegido pelo bloqueio compartilhado da instância.
//
// Servidores sem mode explícito são carregados como observe pelo
// pacote idle.
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

// modeAwareStopper seleciona a autoridade de parada de acordo
// com o mode configurado individualmente para cada servidor.
//
// Somente ServerModeActive pode chegar ao stopper real.
//
// Mode vazio também é tratado como observe como proteção adicional,
// embora o loader de configuração já normalize esse caso.
type modeAwareStopper struct {
	activeStopper idle.ApplicationStopper
}

func newModeAwareStopper(
	activeStopper idle.ApplicationStopper,
) (*modeAwareStopper, error) {
	if activeStopper == nil {
		return nil, fmt.Errorf(
			"o mecanismo de parada do modo active não foi informado",
		)
	}

	return &modeAwareStopper{
		activeStopper: activeStopper,
	}, nil
}

func (s *modeAwareStopper) StopApplication(
	ctx context.Context,
	server idle.Server,
) error {
	if s == nil ||
		s.activeStopper == nil {
		return fmt.Errorf(
			"o mecanismo de parada do modo active não está disponível",
		)
	}

	switch server.Mode {
	case "",
		idle.ServerModeObserve:
		return errIdleObservationOnly

	case idle.ServerModeActive:
		return s.activeStopper.StopApplication(
			ctx,
			server,
		)

	default:
		return fmt.Errorf(
			"o servidor %s possui modo de Idle não reconhecido: %q",
			server.Instance,
			server.Mode,
		)
	}
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

	protectedActiveStopper, err := idle.NewLockedStopper(
		operationManager,
		ampAdapter,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível proteger as paradas automáticas do modo active: %w",
			err,
		)
	}

	configuredStopper, err := newModeAwareStopper(
		protectedActiveStopper,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível configurar a autoridade do motor genérico de Idle: %w",
			err,
		)
	}

	engine, err := idle.NewEngine(
		idleConfig,
		detectors,
		ampAdapter,
		configuredStopper,
		buildIdleEventHandler(
			idleConfig,
			log,
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o motor genérico de Idle: %w",
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
	activeServers := o.config.ActiveServers()
	observeServers := len(enabledServers) - len(activeServers)

	o.log.Info().
		Str("idle_mode", "per_server").
		Str("config_path", idleObserverConfigPath).
		Int("registered_servers", len(o.config.Servers)).
		Int("enabled_servers", len(enabledServers)).
		Int("observe_servers", observeServers).
		Int("active_servers", len(activeServers)).
		Dur("check_interval", o.config.CheckInterval).
		Msg("Motor genérico de Idle iniciado")

	o.engine.Run(
		ctx,
	)

	o.log.Info().
		Str("idle_mode", "per_server").
		Msg("Motor genérico de Idle encerrado")
}

func buildIdleEventHandler(
	config idle.Config,
	log zerolog.Logger,
) idle.EventHandler {
	return func(
		_ context.Context,
		event idle.Event,
	) {
		mode := idle.ServerModeObserve

		if server, exists := config.FindServer(
			event.Instance,
		); exists {
			if server.Mode != "" {
				mode = server.Mode
			}
		}

		eventLog := log.With().
			Str(
				"idle_mode",
				string(mode),
			).
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
				Msg("Motor de Idle não conseguiu consultar o estado da aplicação")

		case idle.EventRuntimeNotOnline:
			eventLog.Debug().
				Msg("Motor de Idle ignorou aplicação que não está Online")

		case idle.EventStartupGraceStarted:
			eventLog.Info().
				Dur(
					"grace_remaining",
					event.GraceRemaining,
				).
				Msg("Motor de Idle iniciou a proteção após a aplicação ficar Online")

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
				Msg("Motor de Idle mantém a proteção inicial ativa")

		case idle.EventDetectorFailed:
			eventLog.Warn().
				Err(event.Err).
				Msg("Motor de Idle não conseguiu consultar os jogadores")

		case idle.EventPlayersPresent:
			eventLog.Debug().
				Int(
					"players",
					event.PlayerCount,
				).
				Msg("Motor de Idle encontrou jogadores conectados")

		case idle.EventIdleTimerStarted:
			eventLog.Info().
				Dur(
					"idle_remaining",
					event.IdleRemaining,
				).
				Msg("Motor de Idle iniciou o contador individual de inatividade")

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
				Msg("Motor de Idle mantém o contador de inatividade ativo")

		case idle.EventFinalCheckFailed:
			eventLog.Warn().
				Err(event.Err).
				Dur(
					"idle_elapsed",
					event.IdleElapsed,
				).
				Msg("Motor de Idle cancelou a decisão por falha na confirmação final")

		case idle.EventStopCancelledPlayers:
			eventLog.Info().
				Int(
					"players",
					event.PlayerCount,
				).
				Msg("Motor de Idle cancelou a parada porque jogadores entraram")

		case idle.EventStopCancelledState:
			eventLog.Info().
				Msg("Motor de Idle cancelou a parada porque o estado da aplicação mudou")

		case idle.EventStopStarting:
			if mode == idle.ServerModeActive {
				eventLog.Info().
					Dur(
						"idle_elapsed",
						event.IdleElapsed,
					).
					Msg("Modo active: limite atingido; parada automática será tentada")

				return
			}

			eventLog.Info().
				Dur(
					"idle_elapsed",
					event.IdleElapsed,
				).
				Msg("Modo observe: limite atingido; parada automática seria tentada")

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
					Msg("Parada automática adiada porque a instância está ocupada")

				return
			}

			if errors.Is(
				event.Err,
				errIdleObservationOnly,
			) {
				eventLog.Debug().
					Msg("Modo observe: Core.Stop foi intencionalmente bloqueado")

				return
			}

			eventLog.Error().
				Err(event.Err).
				Msg("Motor de Idle não conseguiu concluir a parada automática")

		case idle.EventStopSucceeded:
			if mode == idle.ServerModeActive {
				eventLog.Info().
					Msg("Modo active: parada automática concluída")

				return
			}

			eventLog.Warn().
				Msg("Modo observe registrou uma parada concluída inesperadamente")

		default:
			eventLog.Debug().
				Str(
					"event_type",
					string(event.Type),
				).
				Msg("Evento do motor genérico de Idle")
		}
	}
}
