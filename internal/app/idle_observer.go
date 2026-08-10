package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	discordClient "github.com/alabamaamp/ampcontrol/internal/discord"
	"github.com/alabamaamp/ampcontrol/internal/idle"
	"github.com/alabamaamp/ampcontrol/internal/operation"
	"github.com/rs/zerolog"
)

const idleObserverConfigPath = "config/idle.json"
const idleAdditionalServersPath = "data/idle_servers.json"
const idleDetectionOverridesPath = "data/idle_detection_overrides.json"
const idleObserverStatePath = "data/idle_state.json"

var errIdleObservationOnly = errors.New(
	"modo de observação: Core.Stop não foi executado",
)

type idleObserver struct {
	engine         *idle.Engine
	config         idle.Config
	configMu       sync.RWMutex
	registrationMu sync.Mutex
	configChanged  func(idle.Config)
	healthMu       sync.RWMutex
	running        bool
	startedAt      time.Time
	lastEventAt    time.Time
	lastErrorAt    time.Time
	lastError      string
	log            zerolog.Logger
}

type observationOnlyStopper struct{}

func (
	observationOnlyStopper,
) StopApplication(
	context.Context,
	idle.Server,
) error {
	return errIdleObservationOnly
}

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
	notifier idleNotifier,
	idleConfig idle.Config,
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

	if notifier == nil {
		return nil, fmt.Errorf(
			"o notificante do observador genérico de Idle não foi informado",
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

	ampPlayersDetector, err := idle.NewAMPPlayersDetector(
		ampAdapter,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o detector de jogadores da API AMP: %w",
			err,
		)
	}

	detectors, err := idle.NewDefaultDetectorRegistry(
		ampPlayersDetector,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o registro de detectores do observador de Idle: %w",
			err,
		)
	}

	notificationStopper, err := newIdleNotificationStopper(
		ampAdapter,
		notifier,
		log,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível configurar as notificações das paradas automáticas: %w",
			err,
		)
	}

	protectedActiveStopper, err := idle.NewLockedStopper(
		operationManager,
		notificationStopper,
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

	observer := &idleObserver{
		config: idleConfig,
		log:    log,
	}

	engine, err := idle.NewEngine(
		idleConfig,
		detectors,
		ampAdapter,
		configuredStopper,
		buildIdleEventHandler(
			observer.serverMode,
			observer.recordEvent,
			log,
		),
		idle.WithStatePath(idleObserverStatePath),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível criar o motor genérico de Idle: %w",
			err,
		)
	}

	observer.engine = engine
	return observer, nil
}

func (o *idleObserver) Run(
	ctx context.Context,
) {
	if o == nil ||
		o.engine == nil ||
		ctx == nil {
		return
	}

	config := o.configSnapshot()
	enabledServers := config.EnabledServers()
	activeServers := config.ActiveServers()
	observeServers := len(enabledServers) - len(activeServers)

	o.healthMu.Lock()
	o.running = true
	o.startedAt = time.Now()
	o.healthMu.Unlock()
	defer func() {
		o.healthMu.Lock()
		o.running = false
		o.healthMu.Unlock()
	}()

	o.log.Info().
		Str("idle_mode", "per_server").
		Str("config_path", idleObserverConfigPath).
		Int("registered_servers", len(config.Servers)).
		Int("enabled_servers", len(enabledServers)).
		Int("observe_servers", observeServers).
		Int("active_servers", len(activeServers)).
		Dur("check_interval", config.CheckInterval).
		Msg("Motor genérico de Idle iniciado")

	o.engine.Run(
		ctx,
	)

	o.log.Info().
		Str("idle_mode", "per_server").
		Msg("Motor genérico de Idle encerrado")
}

func buildIdleEventHandler(
	resolveMode func(string) idle.ServerMode,
	recordEvent func(idle.Event),
	log zerolog.Logger,
) idle.EventHandler {
	return func(
		_ context.Context,
		event idle.Event,
	) {
		if recordEvent != nil {
			recordEvent(event)
		}

		mode := idle.ServerModeObserve

		if resolveMode != nil {
			if resolved := resolveMode(event.Instance); resolved != "" {
				mode = resolved
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

		case idle.EventStatePersistenceFailed:
			eventLog.Error().
				Err(event.Err).
				Msg("Motor de Idle nao conseguiu persistir os contadores")

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

func (o *idleObserver) recordEvent(event idle.Event) {
	if o == nil {
		return
	}

	now := time.Now()
	o.healthMu.Lock()
	o.lastEventAt = now
	if idleEventIsDiagnosticFailure(event) {
		o.lastErrorAt = now
		o.lastError = strings.TrimSpace(event.Err.Error())
	}
	o.healthMu.Unlock()
}

func idleEventIsDiagnosticFailure(event idle.Event) bool {
	if event.Err == nil {
		return false
	}

	switch event.Type {
	case idle.EventRuntimeUnavailable,
		idle.EventDetectorFailed,
		idle.EventFinalCheckFailed,
		idle.EventStatePersistenceFailed:
		return true

	case idle.EventStopFailed:
		var busyError *idle.OperationBusyError
		return !errors.As(event.Err, &busyError) &&
			!errors.Is(event.Err, errIdleObservationOnly)

	default:
		return false
	}
}

func (o *idleObserver) IdleDiagnostics() discordClient.IdleDiagnosticsSnapshot {
	if o == nil {
		return discordClient.IdleDiagnosticsSnapshot{}
	}

	config := o.configSnapshot()
	enabled := config.EnabledServers()
	active := config.ActiveServers()
	rconServers := 0
	rconReady := 0
	for _, server := range enabled {
		if !idleServerUsesRCON(server) {
			continue
		}
		rconServers++
		if strings.TrimSpace(server.RCONAddress) != "" &&
			strings.TrimSpace(server.RCONPasswordEnv) != "" &&
			strings.TrimSpace(os.Getenv(server.RCONPasswordEnv)) != "" {
			rconReady++
		}
	}

	o.healthMu.RLock()
	snapshot := discordClient.IdleDiagnosticsSnapshot{
		Running:           o.running,
		StartedAt:         o.startedAt,
		LastEventAt:       o.lastEventAt,
		LastErrorAt:       o.lastErrorAt,
		LastError:         o.lastError,
		CheckInterval:     config.CheckInterval,
		RegisteredServers: len(config.Servers),
		EnabledServers:    len(enabled),
		ActiveServers:     len(active),
		ObserveServers:    len(enabled) - len(active),
		RCONServers:       rconServers,
		RCONReadyServers:  rconReady,
	}
	o.healthMu.RUnlock()

	return snapshot
}

func idleServerUsesRCON(server idle.Server) bool {
	return strings.HasSuffix(string(server.Detector), "_rcon") ||
		strings.HasSuffix(string(server.FallbackDetector), "_rcon")
}

func (o *idleObserver) configSnapshot() idle.Config {
	o.configMu.RLock()
	defer o.configMu.RUnlock()

	return idle.Config{
		CheckInterval: o.config.CheckInterval,
		Servers:       append([]idle.Server(nil), o.config.Servers...),
	}
}

func (o *idleObserver) SetConfigChangedHandler(handler func(idle.Config)) {
	if o == nil {
		return
	}
	o.registrationMu.Lock()
	o.configChanged = handler
	o.registrationMu.Unlock()
}

func (o *idleObserver) publishConfig(next idle.Config) {
	o.configMu.Lock()
	o.config = next
	o.configMu.Unlock()
	if o.configChanged != nil {
		o.configChanged(next)
	}
}

func (o *idleObserver) serverMode(instance string) idle.ServerMode {
	config := o.configSnapshot()
	server, exists := config.FindServer(instance)
	if !exists {
		return ""
	}
	return server.Mode
}

func (o *idleObserver) RegisterIdleServer(
	_ context.Context,
	instance amp.ManagedInstance,
) error {
	if o == nil || o.engine == nil {
		return fmt.Errorf("o motor de Idle não está disponível")
	}

	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()

	_, err := idle.RegisterAdditionalServer(
		idleObserverConfigPath,
		idleAdditionalServersPath,
		idle.ServerRegistration{
			Instance:    instance.Name,
			DisplayName: instance.FriendlyName,
			Game:        instance.Game,
		},
	)
	if err != nil {
		return err
	}
	next, err := idle.LoadCombinedWithDetectionOverrides(
		idleObserverConfigPath,
		idleAdditionalServersPath,
		idleDetectionOverridesPath,
	)
	if err != nil {
		return fmt.Errorf("o cadastro foi salvo, mas a configuração combinada falhou: %w", err)
	}
	if err := o.engine.ReplaceConfig(next); err != nil {
		return fmt.Errorf(
			"o cadastro foi salvo, mas não pôde ser aplicado ao motor: %w",
			err,
		)
	}

	o.publishConfig(next)

	o.log.Info().
		Str("server", instance.Name).
		Str("display_name", instance.FriendlyName).
		Str("game", instance.Game).
		Msg("Instância adicionada ao motor de Idle em tempo real")

	return nil
}

func (o *idleObserver) IdleDetectionSettings(
	instance string,
) (discordClient.IdleDetectionSettings, error) {
	server, exists := o.configSnapshot().FindServer(instance)
	if !exists {
		return discordClient.IdleDetectionSettings{}, fmt.Errorf("a instância %s não está cadastrada no Idle", instance)
	}
	return discordClient.IdleDetectionSettings{
		Method:           idleDetectionMethod(server),
		Detector:         string(server.Detector),
		FallbackDetector: string(server.FallbackDetector),
		RCONConfigured: strings.TrimSpace(server.RCONAddress) != "" &&
			strings.TrimSpace(server.RCONPasswordEnv) != "",
	}, nil
}

func (o *idleObserver) SetIdleDetectionMethod(
	_ context.Context,
	instance string,
	method string,
) (discordClient.IdleDetectionSettings, error) {
	if o == nil || o.engine == nil {
		return discordClient.IdleDetectionSettings{}, fmt.Errorf("o motor de Idle não está disponível")
	}

	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()

	detector, fallback, err := parseIdleDetectionMethod(method)
	if err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	current := o.configSnapshot()
	next, err := idle.ApplyDetectionOverrides(current, []idle.DetectionOverride{{
		Instance:         instance,
		Detector:         detector,
		FallbackDetector: fallback,
	}})
	if err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	if err := o.engine.ReplaceConfig(next); err != nil {
		return discordClient.IdleDetectionSettings{}, fmt.Errorf("não foi possível aplicar o detector ao motor: %w", err)
	}
	if err := idle.SetDetectionOverride(
		idleDetectionOverridesPath,
		idle.DetectionOverride{
			Instance:         instance,
			Detector:         detector,
			FallbackDetector: fallback,
		},
	); err != nil {
		_ = o.engine.ReplaceConfig(current)
		return discordClient.IdleDetectionSettings{}, err
	}

	o.publishConfig(next)
	settings, _ := o.IdleDetectionSettings(instance)
	o.log.Info().
		Str("server", instance).
		Str("detector", string(detector)).
		Str("fallback_detector", string(fallback)).
		Msg("Método de detecção atualizado em tempo real")
	return settings, nil
}

func (o *idleObserver) ResetIdleDetectionMethod(
	_ context.Context,
	instance string,
) (discordClient.IdleDetectionSettings, error) {
	if o == nil || o.engine == nil {
		return discordClient.IdleDetectionSettings{}, fmt.Errorf("o motor de Idle não está disponível")
	}

	o.registrationMu.Lock()
	defer o.registrationMu.Unlock()

	base, err := idle.LoadCombined(idleObserverConfigPath, idleAdditionalServersPath)
	if err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	if _, exists := base.FindServer(instance); !exists {
		return discordClient.IdleDetectionSettings{}, fmt.Errorf("a instância %s não está cadastrada no Idle", instance)
	}
	overrides, err := idle.LoadDetectionOverrides(idleDetectionOverridesPath)
	if err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	filtered := make([]idle.DetectionOverride, 0, len(overrides))
	for _, override := range overrides {
		if !strings.EqualFold(override.Instance, instance) {
			filtered = append(filtered, override)
		}
	}
	next, err := idle.ApplyDetectionOverrides(base, filtered)
	if err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	current := o.configSnapshot()
	if err := o.engine.ReplaceConfig(next); err != nil {
		return discordClient.IdleDetectionSettings{}, err
	}
	if _, err := idle.RemoveDetectionOverride(idleDetectionOverridesPath, instance); err != nil {
		_ = o.engine.ReplaceConfig(current)
		return discordClient.IdleDetectionSettings{}, err
	}

	o.publishConfig(next)
	settings, _ := o.IdleDetectionSettings(instance)
	return settings, nil
}

func parseIdleDetectionMethod(method string) (idle.Detector, idle.Detector, error) {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "amp":
		return idle.DetectorAMPPlayers, "", nil
	case "amp_palworld_rcon":
		return idle.DetectorAMPPlayers, idle.DetectorPalworldRCON, nil
	case "amp_project_zomboid_rcon":
		return idle.DetectorAMPPlayers, idle.DetectorProjectZomboidRCON, nil
	default:
		return "", "", fmt.Errorf("o método de detecção %q não é reconhecido", method)
	}
}

func idleDetectionMethod(server idle.Server) string {
	switch server.FallbackDetector {
	case idle.DetectorPalworldRCON:
		return "amp_palworld_rcon"
	case idle.DetectorProjectZomboidRCON:
		return "amp_project_zomboid_rcon"
	default:
		return "amp"
	}
}
