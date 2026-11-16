package discord

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// heartbeatInterval é de quanto em quanto tempo o arquivo de sinal é
// atualizado enquanto o gateway do Discord está conectado.
const heartbeatInterval = 60 * time.Second

// writeHeartbeat grava de forma atômica (arquivo temporário + rename) a hora
// atual em segundos unix.
func writeHeartbeat(path string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary := path + ".tmp"
	content := []byte(strconv.FormatInt(now.Unix(), 10) + "\n")
	if err := os.WriteFile(temporary, content, 0o640); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// runHeartbeat atualiza o arquivo a cada intervalo enquanto connected()
// retornar true; termina com o contexto. O ampcontrol-maintenance heartbeat
// lê este arquivo para saber se o bot está conectado ao Discord.
func runHeartbeat(
	ctx context.Context,
	path string,
	interval time.Duration,
	connected func() bool,
	onError func(error),
) {
	tick := func() {
		if !connected() {
			return
		}
		if err := writeHeartbeat(path, time.Now()); err != nil && onError != nil {
			onError(err)
		}
	}
	tick()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}
