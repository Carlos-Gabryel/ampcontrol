package discord

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestWriteHeartbeatIsAtomicUnixTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord-heartbeat")
	now := time.Unix(1700000000, 0)
	if err := writeHeartbeat(path, now); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != strconv.FormatInt(now.Unix(), 10) {
		t.Fatalf("conteúdo inesperado: %q", got)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("arquivo temporário deveria ter sumido: %v", err)
	}
}

func TestRunHeartbeatOnlyWritesWhenConnected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hb")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runHeartbeat(ctx, path, 10*time.Millisecond, func() bool { return false }, nil)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("não deveria gravar desconectado: %v", err)
	}
	cancel()
	<-done

	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		runHeartbeat(ctx2, path, 10*time.Millisecond, func() bool { return true }, nil)
		close(done2)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("arquivo não foi gravado")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel2()
	<-done2
}
