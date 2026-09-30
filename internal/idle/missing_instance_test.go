package idle

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
)

func TestAMPAdapterMarksInstanceMissingFromInventory(t *testing.T) {
	t.Parallel()

	adapter := newAMPAdapterForTest(
		t,
		&fakeAMPApplicationClient{},
		[]amp.ManagedInstance{{Name: "KalagaPal01", APIURL: "http://127.0.0.1:8088/", Running: true}},
	)

	_, err := adapter.RuntimeState(context.Background(), Server{Instance: "AIO01"})
	if !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("instância ausente de um inventário válido deveria ser ErrInstanceNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "não foi encontrada") {
		t.Fatalf("a mensagem existente deveria ser mantida: %v", err)
	}
}

func TestAMPAdapterDoesNotMarkMissingOnEmptyOrFailedInventory(t *testing.T) {
	t.Parallel()

	empty := newAMPAdapterForTest(t, &fakeAMPApplicationClient{}, nil)
	_, err := empty.RuntimeState(context.Background(), Server{Instance: "AIO01"})
	if err == nil || errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("inventário vazio não prova que a instância foi apagada: %v", err)
	}

	failing, buildErr := newAMPAdapter(
		&fakeAMPApplicationClient{},
		func(context.Context) ([]amp.ManagedInstance, error) {
			return nil, errors.New("ADS fora do ar")
		},
	)
	if buildErr != nil {
		t.Fatalf("não foi possível criar o adaptador: %v", buildErr)
	}
	_, err = failing.RuntimeState(context.Background(), Server{Instance: "AIO01"})
	if err == nil || errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("falha no inventário não prova que a instância foi apagada: %v", err)
	}
}

func missingInstanceError() error {
	return fmt.Errorf("não foi possível consultar: %w", ErrInstanceNotFound)
}

func TestEngineSilencesInstanceMissingFromAMP(t *testing.T) {
	t.Parallel()

	provider := &scriptedRuntimeProvider{errors: make([]error, 100)}
	for index := range provider.errors {
		provider.errors[index] = missingInstanceError()
	}
	engine := newEngineForTest(t, &scriptedEngineDetector{defaultCount: 0}, provider, &fakeApplicationStopper{})
	currentTime := time.Date(2026, time.September, 30, 2, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return currentTime }

	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
	currentTime = currentTime.Add(instanceMissingGrace - time.Second)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)

	currentTime = currentTime.Add(time.Second)
	events := engine.CheckNow(context.Background())
	assertSingleEventType(t, events, EventInstanceMissing)
	if !errors.Is(events[0].Err, ErrInstanceNotFound) {
		t.Fatalf("o evento deveria carregar o motivo: %v", events[0].Err)
	}

	for range 3 {
		currentTime = currentTime.Add(30 * time.Second)
		if events := engine.CheckNow(context.Background()); len(events) != 0 {
			t.Fatalf("instância já reportada como ausente não deveria gerar eventos: %#v", events)
		}
	}
}

func TestEngineOtherFailuresDoNotAdvanceMissingInstance(t *testing.T) {
	t.Parallel()

	provider := &scriptedRuntimeProvider{errors: []error{
		missingInstanceError(),
		errors.New("ADS fora do ar"),
		errors.New("ADS fora do ar"),
	}}
	engine := newEngineForTest(t, &scriptedEngineDetector{defaultCount: 0}, provider, &fakeApplicationStopper{})
	currentTime := time.Date(2026, time.September, 30, 2, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return currentTime }

	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
	currentTime = currentTime.Add(instanceMissingGrace)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)

	currentTime = currentTime.Add(time.Minute)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
	// A instância reaparece: o relógio de ausência precisa recomeçar.
	currentTime = currentTime.Add(time.Minute)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventStartupGraceStarted)

	provider.errors = append(provider.errors, nil, missingInstanceError())
	currentTime = currentTime.Add(time.Minute)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
}

func TestEngineReportsInstanceBackInAMP(t *testing.T) {
	t.Parallel()

	provider := &scriptedRuntimeProvider{errors: []error{missingInstanceError(), missingInstanceError()}}
	engine := newEngineForTest(t, &scriptedEngineDetector{defaultCount: 0}, provider, &fakeApplicationStopper{})
	currentTime := time.Date(2026, time.September, 30, 2, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return currentTime }

	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
	currentTime = currentTime.Add(instanceMissingGrace)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventInstanceMissing)

	currentTime = currentTime.Add(30 * time.Second)
	events := engine.CheckNow(context.Background())
	if len(events) != 2 || events[0].Type != EventInstanceBack || events[1].Type != EventStartupGraceStarted {
		t.Fatalf("a instância de volta deveria ser anunciada e monitorada normalmente: %#v", events)
	}

	provider.errors = append(provider.errors, nil, missingInstanceError())
	currentTime = currentTime.Add(30 * time.Second)
	assertSingleEventType(t, engine.CheckNow(context.Background()), EventRuntimeUnavailable)
}
