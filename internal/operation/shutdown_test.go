package operation

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownWithoutActiveOperationsReturnsImmediately(t *testing.T) {
	t.Parallel()

	manager := NewManager()

	remaining, err := manager.Shutdown(context.Background())
	if err != nil || len(remaining) != 0 {
		t.Fatalf("encerramento sem operações: restantes=%v erro=%v", remaining, err)
	}
	if _, err := manager.TryAcquire("Palworld01", "teste"); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("depois do encerramento, novas operações deveriam ser recusadas: %v", err)
	}
}

func TestShutdownWaitsForRunningOperation(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	result, err := manager.TryAcquire("Palworld01", "comando /amp atualizar")
	if err != nil || !result.Acquired() {
		t.Fatalf("não foi possível reservar a instância: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := manager.Shutdown(context.Background())
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("o encerramento não esperou a operação em andamento: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	waitUntilClosing(t, manager)
	if _, err := manager.TryAcquire("Valheim01", "teste"); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("durante o encerramento, novas operações deveriam ser recusadas: %v", err)
	}

	result.Lease.Release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("encerramento retornou erro: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("o encerramento não terminou depois que a operação acabou")
	}
}

func TestShutdownReportsOperationsLeftWhenDeadlineExpires(t *testing.T) {
	t.Parallel()

	manager := NewManager()
	if _, err := manager.TryAcquire("Palworld01", "comando /amp atualizar"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	remaining, err := manager.Shutdown(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("era esperado prazo esgotado: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Instance != "Palworld01" || remaining[0].Operation != "comando /amp atualizar" {
		t.Fatalf("operações restantes inesperadas: %+v", remaining)
	}
}

func waitUntilClosing(t *testing.T, manager *Manager) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		closing := manager.closing
		manager.mu.Unlock()
		if closing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("o gerenciador não entrou em encerramento")
}
