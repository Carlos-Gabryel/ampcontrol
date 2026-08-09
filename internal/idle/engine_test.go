package idle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedRuntimeProvider struct {
	states       []RuntimeState
	errors       []error
	defaultState RuntimeState
	calls        int
}

func (p *scriptedRuntimeProvider) RuntimeState(
	context.Context,
	Server,
) (RuntimeState, error) {
	index := p.calls
	p.calls++

	if index < len(p.errors) &&
		p.errors[index] != nil {
		return RuntimeStateUnknown, p.errors[index]
	}

	if index < len(p.states) {
		return p.states[index], nil
	}

	if p.defaultState == "" {
		return RuntimeStateOnline, nil
	}

	return p.defaultState, nil
}

type scriptedEngineDetector struct {
	counts       []int
	errors       []error
	defaultCount int
	calls        int
}

func (
	*scriptedEngineDetector,
) DetectorType() Detector {
	return DetectorPalworldRCON
}

func (d *scriptedEngineDetector) PlayerCount(
	context.Context,
	Server,
) (int, error) {
	index := d.calls
	d.calls++

	if index < len(d.errors) &&
		d.errors[index] != nil {
		return 0, d.errors[index]
	}

	if index < len(d.counts) {
		return d.counts[index], nil
	}

	return d.defaultCount, nil
}

type fakeApplicationStopper struct {
	calls int
	err   error
}

func (s *fakeApplicationStopper) StopApplication(
	context.Context,
	Server,
) error {
	s.calls++

	return s.err
}

func TestNewEngineRejectsMissingDependencies(
	t *testing.T,
) {
	t.Parallel()

	config := engineTestConfig()

	_, err := NewEngine(
		config,
		nil,
		&scriptedRuntimeProvider{},
		&fakeApplicationStopper{},
		nil,
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para registro de detectores ausente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"registro de detectores",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestEngineAppliesStartupGraceBeforeDetector(
	t *testing.T,
) {
	t.Parallel()

	baseTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	detector := &scriptedEngineDetector{}

	provider := &scriptedRuntimeProvider{
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := baseTime

	engine.now = func() time.Time {
		return currentTime
	}

	events := engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventStartupGraceStarted,
	)

	currentTime = currentTime.Add(
		4 * time.Minute,
	)

	events = engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventStartupGraceActive,
	)

	if detector.calls != 0 {
		t.Fatalf(
			"o detector não deveria ser chamado durante a proteção inicial; chamadas: %d",
			detector.calls,
		)
	}

	if stopper.calls != 0 {
		t.Fatalf(
			"o servidor não deveria ser parado durante a proteção inicial; chamadas: %d",
			stopper.calls,
		)
	}
}

func TestEngineStopsAfterGraceTimeoutAndFinalChecks(
	t *testing.T,
) {
	t.Parallel()

	baseTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	detector := &scriptedEngineDetector{
		counts: []int{
			0,
			0,
			0,
		},
	}

	provider := &scriptedRuntimeProvider{
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := baseTime

	engine.now = func() time.Time {
		return currentTime
	}

	assertSingleEventType(
		t,
		engine.CheckNow(
			context.Background(),
		),
		EventStartupGraceStarted,
	)

	currentTime = currentTime.Add(
		5 * time.Minute,
	)

	assertSingleEventType(
		t,
		engine.CheckNow(
			context.Background(),
		),
		EventIdleTimerStarted,
	)

	currentTime = currentTime.Add(
		10 * time.Minute,
	)

	events := engine.CheckNow(
		context.Background(),
	)

	if len(events) != 2 {
		t.Fatalf(
			"eram esperados 2 eventos na parada, mas foram recebidos %d: %#v",
			len(events),
			events,
		)
	}

	if events[0].Type != EventStopStarting {
		t.Fatalf(
			"primeiro evento inesperado: %q",
			events[0].Type,
		)
	}

	if events[1].Type != EventStopSucceeded {
		t.Fatalf(
			"segundo evento inesperado: %q",
			events[1].Type,
		)
	}

	if detector.calls != 3 {
		t.Fatalf(
			"eram esperadas 3 consultas ao detector, mas ocorreram %d",
			detector.calls,
		)
	}

	if provider.calls != 4 {
		t.Fatalf(
			"eram esperadas 4 consultas de estado, incluindo a confirmação final, mas ocorreram %d",
			provider.calls,
		)
	}

	if stopper.calls != 1 {
		t.Fatalf(
			"era esperada 1 parada, mas ocorreram %d",
			stopper.calls,
		)
	}
}

func TestEngineCancelsStopWhenPlayerJoinsFinalCheck(
	t *testing.T,
) {
	t.Parallel()

	detector := &scriptedEngineDetector{
		counts: []int{
			0,
			0,
			2,
		},
	}

	provider := &scriptedRuntimeProvider{
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	engine.now = func() time.Time {
		return currentTime
	}

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		5 * time.Minute,
	)

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		10 * time.Minute,
	)

	events := engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventStopCancelledPlayers,
	)

	if events[0].PlayerCount != 2 {
		t.Fatalf(
			"quantidade inesperada de jogadores na confirmação: %d",
			events[0].PlayerCount,
		)
	}

	if stopper.calls != 0 {
		t.Fatalf(
			"o servidor não deveria ser parado; chamadas: %d",
			stopper.calls,
		)
	}
}

func TestEngineFailsOpenWhenDetectorFails(
	t *testing.T,
) {
	t.Parallel()

	detectorErr := errors.New(
		"RCON indisponível",
	)

	detector := &scriptedEngineDetector{
		counts: []int{
			0,
			0,
			0,
		},
		errors: []error{
			nil,
			detectorErr,
			nil,
		},
	}

	provider := &scriptedRuntimeProvider{
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	engine.now = func() time.Time {
		return currentTime
	}

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		5 * time.Minute,
	)

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		10 * time.Minute,
	)

	events := engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventDetectorFailed,
	)

	if !errors.Is(
		events[0].Err,
		detectorErr,
	) {
		t.Fatalf(
			"o evento não contém o erro esperado: %v",
			events[0].Err,
		)
	}

	if stopper.calls != 0 {
		t.Fatalf(
			"uma falha do detector nunca deve parar o servidor; chamadas: %d",
			stopper.calls,
		)
	}

	currentTime = currentTime.Add(
		time.Minute,
	)

	events = engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventIdleTimerStarted,
	)
}

func TestEngineCancelsStopWhenRuntimeChanges(
	t *testing.T,
) {
	t.Parallel()

	detector := &scriptedEngineDetector{
		counts: []int{
			0,
			0,
			0,
		},
	}

	provider := &scriptedRuntimeProvider{
		states: []RuntimeState{
			RuntimeStateOnline,
			RuntimeStateOnline,
			RuntimeStateOnline,
			RuntimeStateBusy,
		},
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	engine.now = func() time.Time {
		return currentTime
	}

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		5 * time.Minute,
	)

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		10 * time.Minute,
	)

	events := engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventStopCancelledState,
	)

	if events[0].RuntimeState != RuntimeStateBusy {
		t.Fatalf(
			"estado final inesperado: %q",
			events[0].RuntimeState,
		)
	}

	if stopper.calls != 0 {
		t.Fatalf(
			"o servidor não deveria ser parado durante uma transição; chamadas: %d",
			stopper.calls,
		)
	}
}

func TestEngineReportsStopFailureAndResetsTimer(
	t *testing.T,
) {
	t.Parallel()

	stopErr := errors.New(
		"Core.Stop falhou",
	)

	detector := &scriptedEngineDetector{
		defaultCount: 0,
	}

	provider := &scriptedRuntimeProvider{
		defaultState: RuntimeStateOnline,
	}

	stopper := &fakeApplicationStopper{
		err: stopErr,
	}

	engine := newEngineForTest(
		t,
		detector,
		provider,
		stopper,
	)

	currentTime := time.Date(
		2026,
		time.August,
		7,
		1,
		30,
		0,
		0,
		time.UTC,
	)

	engine.now = func() time.Time {
		return currentTime
	}

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		5 * time.Minute,
	)

	engine.CheckNow(
		context.Background(),
	)

	currentTime = currentTime.Add(
		10 * time.Minute,
	)

	events := engine.CheckNow(
		context.Background(),
	)

	if len(events) != 2 ||
		events[0].Type != EventStopStarting ||
		events[1].Type != EventStopFailed {
		t.Fatalf(
			"eventos inesperados: %#v",
			events,
		)
	}

	if !errors.Is(
		events[1].Err,
		stopErr,
	) {
		t.Fatalf(
			"o evento não contém o erro esperado: %v",
			events[1].Err,
		)
	}

	currentTime = currentTime.Add(
		time.Minute,
	)

	events = engine.CheckNow(
		context.Background(),
	)

	assertSingleEventType(
		t,
		events,
		EventIdleTimerStarted,
	)
}

func engineTestConfig() Config {
	return Config{
		CheckInterval: 30 * time.Second,
		Servers: []Server{
			{
				Instance:     "AlamamaPal01",
				DisplayName:  "Alamama",
				Enabled:      true,
				Detector:     DetectorPalworldRCON,
				IdleTimeout:  10 * time.Minute,
				StartupGrace: 5 * time.Minute,
			},
		},
	}
}

func newEngineForTest(
	t *testing.T,
	detector *scriptedEngineDetector,
	provider *scriptedRuntimeProvider,
	stopper *fakeApplicationStopper,
) *Engine {
	t.Helper()

	registry, err := NewDetectorRegistry(
		detector,
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o registro de teste: %v",
			err,
		)
	}

	engine, err := NewEngine(
		engineTestConfig(),
		registry,
		provider,
		stopper,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"não foi possível criar o motor de teste: %v",
			err,
		)
	}

	return engine
}

func assertSingleEventType(
	t *testing.T,
	events []Event,
	expected EventType,
) {
	t.Helper()

	if len(events) != 1 {
		t.Fatalf(
			"era esperado 1 evento, mas foram recebidos %d: %#v",
			len(events),
			events,
		)
	}

	if events[0].Type != expected {
		t.Fatalf(
			"evento inesperado: obtido %q, esperado %q",
			events[0].Type,
			expected,
		)
	}
}
