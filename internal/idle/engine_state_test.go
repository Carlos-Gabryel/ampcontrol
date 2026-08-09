package idle

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

type observedRuntimeProvider struct {
	now       *time.Time
	startedAt time.Time
	state     RuntimeState
}

func (p *observedRuntimeProvider) RuntimeState(
	context.Context,
	Server,
) (RuntimeState, error) {
	return p.state, nil
}

func (p *observedRuntimeProvider) RuntimeObservation(
	context.Context,
	Server,
) (RuntimeObservation, error) {
	observation := RuntimeObservation{State: p.state}
	if p.state == RuntimeStateOnline && p.now != nil && !p.startedAt.IsZero() {
		observation.Uptime = p.now.Sub(p.startedAt)
		observation.UptimeKnown = true
	}

	return observation, nil
}

func TestEngineRestoresIdleTimerForSameGameProcess(t *testing.T) {
	baseTime := time.Date(2026, time.August, 9, 1, 0, 0, 0, time.UTC)
	currentTime := baseTime
	startedAt := baseTime.Add(-time.Hour)
	statePath := filepath.Join(t.TempDir(), "data", "idle_state.json")

	firstStopper := &fakeApplicationStopper{}
	firstEngine := newPersistentEngineForTest(
		t,
		&currentTime,
		startedAt,
		statePath,
		firstStopper,
	)

	assertSingleEventType(
		t,
		firstEngine.CheckNow(context.Background()),
		EventStartupGraceStarted,
	)

	currentTime = baseTime.Add(5 * time.Minute)
	assertSingleEventType(
		t,
		firstEngine.CheckNow(context.Background()),
		EventIdleTimerStarted,
	)

	currentTime = baseTime.Add(10 * time.Minute)
	assertSingleEventType(
		t,
		firstEngine.CheckNow(context.Background()),
		EventIdleTimerActive,
	)

	secondStopper := &fakeApplicationStopper{}
	secondEngine := newPersistentEngineForTest(
		t,
		&currentTime,
		startedAt,
		statePath,
		secondStopper,
	)

	restoredEvents := secondEngine.CheckNow(context.Background())
	assertSingleEventType(t, restoredEvents, EventIdleTimerActive)
	if restoredEvents[0].IdleElapsed != 5*time.Minute {
		t.Fatalf(
			"tempo restaurado inesperado: %s",
			restoredEvents[0].IdleElapsed,
		)
	}

	currentTime = baseTime.Add(15 * time.Minute)
	stopEvents := secondEngine.CheckNow(context.Background())
	if len(stopEvents) != 2 ||
		stopEvents[0].Type != EventStopStarting ||
		stopEvents[1].Type != EventStopSucceeded {
		t.Fatalf("eventos de parada inesperados: %#v", stopEvents)
	}
	if secondStopper.calls != 1 {
		t.Fatalf("quantidade inesperada de paradas: %d", secondStopper.calls)
	}
}

func TestEngineRestoredStateProtectsNewGameProcess(t *testing.T) {
	baseTime := time.Date(2026, time.August, 9, 2, 0, 0, 0, time.UTC)
	currentTime := baseTime
	statePath := filepath.Join(t.TempDir(), "data", "idle_state.json")

	firstEngine := newPersistentEngineForTest(
		t,
		&currentTime,
		baseTime.Add(-time.Hour),
		statePath,
		&fakeApplicationStopper{},
	)
	firstEngine.CheckNow(context.Background())
	currentTime = baseTime.Add(5 * time.Minute)
	firstEngine.CheckNow(context.Background())
	currentTime = baseTime.Add(10 * time.Minute)
	firstEngine.CheckNow(context.Background())

	restartedGameAt := currentTime.Add(-time.Minute)
	secondEngine := newPersistentEngineForTest(
		t,
		&currentTime,
		restartedGameAt,
		statePath,
		&fakeApplicationStopper{},
	)

	assertSingleEventType(
		t,
		secondEngine.CheckNow(context.Background()),
		EventStartupGraceStarted,
	)
}

func newPersistentEngineForTest(
	t *testing.T,
	currentTime *time.Time,
	applicationStartedAt time.Time,
	statePath string,
	stopper *fakeApplicationStopper,
) *Engine {
	t.Helper()

	detector := &scriptedEngineDetector{defaultCount: 0}
	registry, err := NewDetectorRegistry(detector)
	if err != nil {
		t.Fatalf("não foi possível criar o registro: %v", err)
	}

	provider := &observedRuntimeProvider{
		now:       currentTime,
		startedAt: applicationStartedAt,
		state:     RuntimeStateOnline,
	}
	engine, err := NewEngine(
		engineTestConfig(),
		registry,
		provider,
		stopper,
		nil,
		WithStatePath(statePath),
	)
	if err != nil {
		t.Fatalf("não foi possível criar o motor persistente: %v", err)
	}
	engine.now = func() time.Time { return *currentTime }

	return engine
}
