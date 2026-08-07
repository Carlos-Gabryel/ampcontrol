package idle

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// RuntimeState representa o estado simplificado da aplicação
// hospedada pela instância AMP.
type RuntimeState string

const (
	RuntimeStateUnknown RuntimeState = "unknown"
	RuntimeStateOffline RuntimeState = "offline"
	RuntimeStateIdle    RuntimeState = "idle"
	RuntimeStateOnline  RuntimeState = "online"
	RuntimeStateBusy    RuntimeState = "busy"
	RuntimeStateFailed  RuntimeState = "failed"
)

// RuntimeStatusProvider consulta o estado atual do processo do jogo.
type RuntimeStatusProvider interface {
	RuntimeState(
		ctx context.Context,
		server Server,
	) (RuntimeState, error)
}

// ApplicationStopper coloca somente o processo do jogo em Idle.
//
// A instância AMP deve permanecer ligada.
type ApplicationStopper interface {
	StopApplication(
		ctx context.Context,
		server Server,
	) error
}

type EventType string

const (
	EventRuntimeUnavailable   EventType = "runtime_unavailable"
	EventRuntimeNotOnline     EventType = "runtime_not_online"
	EventStartupGraceStarted  EventType = "startup_grace_started"
	EventStartupGraceActive   EventType = "startup_grace_active"
	EventDetectorFailed       EventType = "detector_failed"
	EventPlayersPresent       EventType = "players_present"
	EventIdleTimerStarted     EventType = "idle_timer_started"
	EventIdleTimerActive      EventType = "idle_timer_active"
	EventFinalCheckFailed     EventType = "final_check_failed"
	EventStopCancelledPlayers EventType = "stop_cancelled_players"
	EventStopCancelledState   EventType = "stop_cancelled_state"
	EventStopStarting         EventType = "stop_starting"
	EventStopSucceeded        EventType = "stop_succeeded"
	EventStopFailed           EventType = "stop_failed"
)

type Event struct {
	Type           EventType
	Instance       string
	DisplayName    string
	RuntimeState   RuntimeState
	PlayerCount    int
	IdleElapsed    time.Duration
	IdleRemaining  time.Duration
	GraceElapsed   time.Duration
	GraceRemaining time.Duration
	Err            error
}

type EventHandler func(
	ctx context.Context,
	event Event,
)

type serverTracker struct {
	LastRuntime RuntimeState
	OnlineSince time.Time
	EmptySince  time.Time
}

type Engine struct {
	config         Config
	detectors      *DetectorRegistry
	statusProvider RuntimeStatusProvider
	stopper        ApplicationStopper
	eventHandler   EventHandler
	now            func() time.Time

	mu       sync.Mutex
	trackers map[string]*serverTracker
}

// NewEngine cria o motor genérico de Idle.
//
// O construtor valida todas as dependências antes que o monitor
// possa ser executado.
func NewEngine(
	config Config,
	detectors *DetectorRegistry,
	statusProvider RuntimeStatusProvider,
	stopper ApplicationStopper,
	eventHandler EventHandler,
) (*Engine, error) {
	if config.CheckInterval <= 0 {
		return nil, fmt.Errorf(
			"o intervalo de verificação do motor de Idle precisa ser maior que zero",
		)
	}

	if detectors == nil {
		return nil, fmt.Errorf(
			"o registro de detectores do motor de Idle não foi informado",
		)
	}

	if statusProvider == nil {
		return nil, fmt.Errorf(
			"o provedor de estado das aplicações não foi informado",
		)
	}

	if stopper == nil {
		return nil, fmt.Errorf(
			"o controlador de parada das aplicações não foi informado",
		)
	}

	enabledServers := config.EnabledServers()

	for _, server := range enabledServers {
		if strings.TrimSpace(server.Instance) == "" {
			return nil, fmt.Errorf(
				"há um servidor habilitado sem nome de instância",
			)
		}

		if server.IdleTimeout <= 0 {
			return nil, fmt.Errorf(
				"o tempo de Idle da instância %s precisa ser maior que zero",
				server.Instance,
			)
		}

		if server.StartupGrace <= 0 {
			return nil, fmt.Errorf(
				"a proteção inicial da instância %s precisa ser maior que zero",
				server.Instance,
			)
		}

		if !detectors.Supports(server.Detector) {
			return nil, fmt.Errorf(
				"o detector %q da instância %s não está registrado",
				server.Detector,
				server.Instance,
			)
		}
	}

	return &Engine{
		config:         config,
		detectors:      detectors,
		statusProvider: statusProvider,
		stopper:        stopper,
		eventHandler:   eventHandler,
		now:            time.Now,
		trackers: make(
			map[string]*serverTracker,
			len(enabledServers),
		),
	}, nil
}

// Run executa uma verificação imediatamente e depois repete
// conforme o intervalo definido na configuração.
//
// O método encerra quando o contexto é cancelado.
func (e *Engine) Run(
	ctx context.Context,
) {
	if ctx == nil {
		return
	}

	e.CheckNow(ctx)

	ticker := time.NewTicker(
		e.config.CheckInterval,
	)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			e.CheckNow(ctx)
		}
	}
}

// CheckNow executa um ciclo completo do motor.
//
// O mutex impede que dois ciclos do monitor sejam executados
// simultaneamente.
func (e *Engine) CheckNow(
	ctx context.Context,
) []Event {
	if e == nil || ctx == nil {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	events := make(
		[]Event,
		0,
		len(e.config.Servers),
	)

	for _, server := range e.config.EnabledServers() {
		if ctx.Err() != nil {
			break
		}

		events = append(
			events,
			e.checkServer(
				ctx,
				server,
			)...,
		)
	}

	return events
}

func (e *Engine) checkServer(
	ctx context.Context,
	server Server,
) []Event {
	tracker := e.trackerFor(
		server.Instance,
	)

	now := e.now()

	runtimeState, err := e.statusProvider.RuntimeState(
		ctx,
		server,
	)
	if err != nil {
		e.resetTracker(
			tracker,
			RuntimeStateUnknown,
		)

		return e.emitMany(
			ctx,
			Event{
				Type:         EventRuntimeUnavailable,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateUnknown,
				Err:          err,
			},
		)
	}

	if runtimeState != RuntimeStateOnline {
		e.resetTracker(
			tracker,
			runtimeState,
		)

		return e.emitMany(
			ctx,
			Event{
				Type:         EventRuntimeNotOnline,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: runtimeState,
			},
		)
	}

	if tracker.LastRuntime != RuntimeStateOnline ||
		tracker.OnlineSince.IsZero() {
		tracker.LastRuntime = RuntimeStateOnline
		tracker.OnlineSince = now
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:           EventStartupGraceStarted,
				Instance:       server.Instance,
				DisplayName:    server.DisplayName,
				RuntimeState:   RuntimeStateOnline,
				GraceRemaining: server.StartupGrace,
			},
		)
	}

	graceElapsed := nonNegativeDuration(
		now.Sub(tracker.OnlineSince),
	)

	if graceElapsed < server.StartupGrace {
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:           EventStartupGraceActive,
				Instance:       server.Instance,
				DisplayName:    server.DisplayName,
				RuntimeState:   RuntimeStateOnline,
				GraceElapsed:   graceElapsed,
				GraceRemaining: server.StartupGrace - graceElapsed,
			},
		)
	}

	playerCount, err := e.detectors.PlayerCount(
		ctx,
		server,
	)
	if err != nil {
		// Fail-open:
		// um erro no detector nunca é interpretado como zero jogadores.
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:         EventDetectorFailed,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateOnline,
				Err:          err,
			},
		)
	}

	if playerCount > 0 {
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:         EventPlayersPresent,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateOnline,
				PlayerCount:  playerCount,
			},
		)
	}

	if tracker.EmptySince.IsZero() {
		tracker.EmptySince = now

		return e.emitMany(
			ctx,
			Event{
				Type:          EventIdleTimerStarted,
				Instance:      server.Instance,
				DisplayName:   server.DisplayName,
				RuntimeState:  RuntimeStateOnline,
				PlayerCount:   0,
				IdleRemaining: server.IdleTimeout,
			},
		)
	}

	idleElapsed := nonNegativeDuration(
		now.Sub(tracker.EmptySince),
	)

	if idleElapsed < server.IdleTimeout {
		return e.emitMany(
			ctx,
			Event{
				Type:          EventIdleTimerActive,
				Instance:      server.Instance,
				DisplayName:   server.DisplayName,
				RuntimeState:  RuntimeStateOnline,
				PlayerCount:   0,
				IdleElapsed:   idleElapsed,
				IdleRemaining: server.IdleTimeout - idleElapsed,
			},
		)
	}

	// Primeira confirmação final:
	// consultar novamente os jogadores antes de parar.
	confirmedPlayers, err := e.detectors.PlayerCount(
		ctx,
		server,
	)
	if err != nil {
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:         EventFinalCheckFailed,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateOnline,
				IdleElapsed:  idleElapsed,
				Err:          err,
			},
		)
	}

	if confirmedPlayers > 0 {
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:         EventStopCancelledPlayers,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateOnline,
				PlayerCount:  confirmedPlayers,
				IdleElapsed:  idleElapsed,
			},
		)
	}

	// Segunda confirmação final:
	// confirmar que o processo ainda está Online antes de Core.Stop.
	confirmedState, err := e.statusProvider.RuntimeState(
		ctx,
		server,
	)
	if err != nil {
		tracker.EmptySince = time.Time{}

		return e.emitMany(
			ctx,
			Event{
				Type:         EventFinalCheckFailed,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateUnknown,
				IdleElapsed:  idleElapsed,
				Err:          err,
			},
		)
	}

	if confirmedState != RuntimeStateOnline {
		e.resetTracker(
			tracker,
			confirmedState,
		)

		return e.emitMany(
			ctx,
			Event{
				Type:         EventStopCancelledState,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: confirmedState,
				IdleElapsed:  idleElapsed,
			},
		)
	}

	events := e.emitMany(
		ctx,
		Event{
			Type:         EventStopStarting,
			Instance:     server.Instance,
			DisplayName:  server.DisplayName,
			RuntimeState: RuntimeStateOnline,
			IdleElapsed:  idleElapsed,
		},
	)

	if err := e.stopper.StopApplication(
		ctx,
		server,
	); err != nil {
		// A tentativa falhou. O contador é reiniciado para impedir
		// uma nova tentativa automática a cada ciclo de 30 segundos.
		tracker.EmptySince = time.Time{}

		return append(
			events,
			e.emitMany(
				ctx,
				Event{
					Type:         EventStopFailed,
					Instance:     server.Instance,
					DisplayName:  server.DisplayName,
					RuntimeState: RuntimeStateOnline,
					IdleElapsed:  idleElapsed,
					Err:          err,
				},
			)...,
		)
	}

	e.resetTracker(
		tracker,
		RuntimeStateIdle,
	)

	return append(
		events,
		e.emitMany(
			ctx,
			Event{
				Type:         EventStopSucceeded,
				Instance:     server.Instance,
				DisplayName:  server.DisplayName,
				RuntimeState: RuntimeStateIdle,
				IdleElapsed:  idleElapsed,
			},
		)...,
	)
}

func (e *Engine) trackerFor(
	instance string,
) *serverTracker {
	key := strings.ToLower(
		strings.TrimSpace(instance),
	)

	tracker, exists := e.trackers[key]
	if exists {
		return tracker
	}

	tracker = &serverTracker{
		LastRuntime: RuntimeStateUnknown,
	}

	e.trackers[key] = tracker

	return tracker
}

func (e *Engine) resetTracker(
	tracker *serverTracker,
	runtimeState RuntimeState,
) {
	tracker.LastRuntime = runtimeState
	tracker.OnlineSince = time.Time{}
	tracker.EmptySince = time.Time{}
}

func (e *Engine) emitMany(
	ctx context.Context,
	events ...Event,
) []Event {
	if e.eventHandler != nil {
		for _, event := range events {
			e.eventHandler(
				ctx,
				event,
			)
		}
	}

	return events
}

func nonNegativeDuration(
	duration time.Duration,
) time.Duration {
	if duration < 0 {
		return 0
	}

	return duration
}
