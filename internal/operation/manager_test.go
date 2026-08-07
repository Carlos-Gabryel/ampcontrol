package operation

import (
	"strings"
	"testing"
	"time"
)

func TestTryAcquireRejectsEmptyInstance(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	_, err := manager.TryAcquire(
		"   ",
		"iniciar",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para instância vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"nome da instância",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestTryAcquireRejectsEmptyOperation(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	_, err := manager.TryAcquire(
		"AlamamaPal01",
		"   ",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para operação vazia",
		)
	}

	if !strings.Contains(
		err.Error(),
		"nome da operação",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}

func TestManagerAcquiresAndReleasesInstance(
	t *testing.T,
) {
	t.Parallel()

	startedAt := time.Date(
		2026,
		time.August,
		7,
		2,
		30,
		0,
		0,
		time.UTC,
	)

	manager := newManager(
		func() time.Time {
			return startedAt
		},
	)

	result, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp iniciar",
	)
	if err != nil {
		t.Fatalf(
			"TryAcquire retornou erro: %v",
			err,
		)
	}

	if !result.Acquired() {
		t.Fatalf(
			"o bloqueio deveria ter sido adquirido: %#v",
			result.Active,
		)
	}

	if result.Active != nil {
		t.Fatalf(
			"não deveria existir operação concorrente: %#v",
			result.Active,
		)
	}

	info, exists := manager.Current(
		"AlamamaPal01",
	)

	if !exists {
		t.Fatal(
			"a operação adquirida não foi encontrada",
		)
	}

	if info.Instance != "AlamamaPal01" {
		t.Fatalf(
			"instância inesperada: %q",
			info.Instance,
		)
	}

	if info.Operation != "comando /amp iniciar" {
		t.Fatalf(
			"operação inesperada: %q",
			info.Operation,
		)
	}

	if !info.StartedAt.Equal(startedAt) {
		t.Fatalf(
			"horário inesperado: %s",
			info.StartedAt,
		)
	}

	result.Lease.Release()

	_, exists = manager.Current(
		"AlamamaPal01",
	)

	if exists {
		t.Fatal(
			"a operação deveria ter sido liberada",
		)
	}
}

func TestManagerBlocksSameInstanceCaseInsensitive(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	first, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp reiniciar",
	)
	if err != nil {
		t.Fatalf(
			"primeira aquisição retornou erro: %v",
			err,
		)
	}

	defer first.Lease.Release()

	second, err := manager.TryAcquire(
		"  alamamapal01  ",
		"Idle automático",
	)
	if err != nil {
		t.Fatalf(
			"segunda aquisição retornou erro: %v",
			err,
		)
	}

	if second.Acquired() {
		second.Lease.Release()

		t.Fatal(
			"a mesma instância não deveria aceitar duas operações",
		)
	}

	if second.Active == nil {
		t.Fatal(
			"a operação ativa deveria ter sido informada",
		)
	}

	if second.Active.Operation != "comando /amp reiniciar" {
		t.Fatalf(
			"operação ativa inesperada: %q",
			second.Active.Operation,
		)
	}
}

func TestManagerAllowsDifferentInstances(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	alamama, err := manager.TryAcquire(
		"AlamamaPal01",
		"comando /amp iniciar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir Alamama: %v",
			err,
		)
	}

	defer alamama.Lease.Release()

	kalaga, err := manager.TryAcquire(
		"KalagaPal01",
		"comando /amp parar",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir Kalaga: %v",
			err,
		)
	}

	defer kalaga.Lease.Release()

	if !alamama.Acquired() {
		t.Fatal(
			"Alamama deveria estar adquirida",
		)
	}

	if !kalaga.Acquired() {
		t.Fatal(
			"Kalaga deveria estar adquirida",
		)
	}
}

func TestLeaseReleaseIsIdempotent(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	first, err := manager.TryAcquire(
		"AlamamaPal01",
		"primeira operação",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir a primeira operação: %v",
			err,
		)
	}

	first.Lease.Release()

	second, err := manager.TryAcquire(
		"AlamamaPal01",
		"segunda operação",
	)
	if err != nil {
		t.Fatalf(
			"não foi possível adquirir a segunda operação: %v",
			err,
		)
	}

	defer second.Lease.Release()

	first.Lease.Release()

	info, exists := manager.Current(
		"AlamamaPal01",
	)

	if !exists {
		t.Fatal(
			"a segunda operação foi liberada indevidamente",
		)
	}

	if info.Operation != "segunda operação" {
		t.Fatalf(
			"operação inesperada após a liberação repetida: %q",
			info.Operation,
		)
	}
}

func TestCurrentRejectsEmptyInstance(
	t *testing.T,
) {
	t.Parallel()

	manager := NewManager()

	if _, exists := manager.Current("   "); exists {
		t.Fatal(
			"uma instância vazia não deveria possuir operação ativa",
		)
	}
}

func TestNilManagerReturnsError(
	t *testing.T,
) {
	t.Parallel()

	var manager *Manager

	_, err := manager.TryAcquire(
		"AlamamaPal01",
		"iniciar",
	)

	if err == nil {
		t.Fatal(
			"era esperado um erro para gerenciador nulo",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não foi inicializado",
	) {
		t.Fatalf(
			"erro inesperado: %v",
			err,
		)
	}
}
