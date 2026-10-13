package amp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// installFakeControlSudo coloca um script no lugar do sudo. O script registra
// os argumentos e o ambiente, imprime o conteúdo de "out" e termina com o
// código presente em "code" (0 quando ausente). Se existir "sleep", dorme.
func installFakeControlSudo(t *testing.T) string {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("sudo falso requer shell POSIX/Linux")
	}

	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"d='" + dir + "'\n" +
		"printf '%s\\n' \"$*\" >> \"$d/calls.log\"\n" +
		"printf '%s|%s\\n' \"$TERM\" \"$NO_COLOR\" >> \"$d/env.log\"\n" +
		"[ -e \"$d/sleep\" ] && exec sleep 30\n" +
		"[ -e \"$d/out\" ] && cat \"$d/out\"\n" +
		"code=0\n" +
		"[ -e \"$d/code\" ] && code=$(cat \"$d/code\")\n" +
		"exit \"$code\"\n"

	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0o755); err != nil {
		t.Fatalf("não foi possível criar o sudo falso: %v", err)
	}

	config := RuntimeConfig{
		SystemUser:  "amp",
		ManagerPath: "/usr/bin/ampinstmgr",
		WrapperPath: "/usr/local/bin/ampcontrol-amp",
		SudoPath:    filepath.Join(dir, "sudo"),
	}

	if err := ConfigureRuntime(config); err != nil {
		t.Fatalf("ConfigureRuntime retornou erro: %v", err)
	}

	t.Cleanup(func() {
		config.SudoPath = "/usr/bin/sudo"
		_ = ConfigureRuntime(config)
	})

	return dir
}

func writeControlFake(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("não foi possível escrever %s: %v", name, err)
	}
}

func readControlFake(t *testing.T, dir, name string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, name))
	if os.IsNotExist(err) {
		return ""
	}

	if err != nil {
		t.Fatalf("não foi possível ler %s: %v", name, err)
	}

	return string(data)
}

// ConfigureRuntime altera estado global; os testes abaixo não usam t.Parallel.

func TestControlInstanceBuildsWrapperCommand(t *testing.T) {
	tests := []struct {
		name     string
		call     func(context.Context, string) error
		wantArgs string
	}{
		{
			name:     "iniciar",
			call:     StartInstance,
			wantArgs: "-n -u amp /usr/local/bin/ampcontrol-amp start Minecraft01",
		},
		{
			name:     "parar",
			call:     StopInstance,
			wantArgs: "-n -u amp /usr/local/bin/ampcontrol-amp stop Minecraft01",
		},
		{
			name:     "reiniciar",
			call:     RestartInstance,
			wantArgs: "-n -u amp /usr/local/bin/ampcontrol-amp restart Minecraft01",
		},
		{
			name:     "atualizar",
			call:     UpdateInstance,
			wantArgs: "-n -u amp /usr/local/bin/ampcontrol-amp update Minecraft01",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := installFakeControlSudo(t)

			// O nome com espaços nas pontas deve ser aparado.
			if err := test.call(context.Background(), "  Minecraft01 \n"); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}

			if got := strings.TrimSpace(readControlFake(t, dir, "calls.log")); got != test.wantArgs {
				t.Fatalf("argumentos = %q, esperado %q", got, test.wantArgs)
			}

			if got := strings.TrimSpace(readControlFake(t, dir, "env.log")); got != "dumb|1" {
				t.Fatalf("ambiente TERM|NO_COLOR = %q, esperado %q", got, "dumb|1")
			}
		})
	}
}

func TestControlInstanceUsesConfiguredUserAndWrapper(t *testing.T) {
	dir := installFakeControlSudo(t)

	config := currentRuntimeConfig()
	config.SystemUser = "outro_usuario"
	config.WrapperPath = "/opt/wrapper"

	if err := ConfigureRuntime(config); err != nil {
		t.Fatalf("ConfigureRuntime retornou erro: %v", err)
	}

	if err := ControlInstance(context.Background(), InstanceOperationStart, "Minecraft01"); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	want := "-n -u outro_usuario /opt/wrapper start Minecraft01"
	if got := strings.TrimSpace(readControlFake(t, dir, "calls.log")); got != want {
		t.Fatalf("argumentos = %q, esperado %q", got, want)
	}
}

func TestControlInstanceValidationNeverRunsWrapper(t *testing.T) {
	tests := []struct {
		name      string
		operation InstanceOperation
		instance  string
	}{
		{"nome vazio", InstanceOperationStart, ""},
		{"nome só com espaços", InstanceOperationStart, " \t"},
		{"ADS01 protegida", InstanceOperationStop, "ADS01"},
		{"ADS01 em minúsculas", InstanceOperationStop, "ads01"},
		{"ADS01 com espaços", InstanceOperationRestart, "  Ads01 "},
		{"operação inválida", InstanceOperation("delete"), "Minecraft01"},
		{"operação vazia", InstanceOperation(""), "Minecraft01"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := installFakeControlSudo(t)

			if err := ControlInstance(context.Background(), test.operation, test.instance); err == nil {
				t.Fatal("esperava erro de validação")
			}

			if got := readControlFake(t, dir, "calls.log"); got != "" {
				t.Fatalf("o wrapper não deveria ser executado, mas registrou %q", got)
			}
		})
	}
}

func TestControlInstanceWrapperFailure(t *testing.T) {
	t.Run("código de saída e saída limpa", func(t *testing.T) {
		dir := installFakeControlSudo(t)
		writeControlFake(t, dir, "code", "3")
		writeControlFake(t, dir, "out", "\x1b[31mfalha vermelha\x1b[0m\n\x1b]0;título\x07│ detalhe │\n")

		err := ControlInstance(context.Background(), InstanceOperationRestart, "Minecraft01")
		if err == nil {
			t.Fatal("esperava erro")
		}

		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("o erro deveria encapsular *exec.ExitError: %v", err)
		}

		if exitErr.ExitCode() != 3 {
			t.Fatalf("código de saída = %d, esperado 3", exitErr.ExitCode())
		}

		message := err.Error()
		for _, want := range []string{"restart", "Minecraft01", "falha vermelha", "│ detalhe │"} {
			if !strings.Contains(message, want) {
				t.Fatalf("erro %q deveria conter %q", message, want)
			}
		}

		for _, unwanted := range []string{"\x1b", "título", "[31m"} {
			if strings.Contains(message, unwanted) {
				t.Fatalf("erro %q não deveria conter %q", message, unwanted)
			}
		}
	})

	t.Run("saída vazia usa mensagem padrão", func(t *testing.T) {
		dir := installFakeControlSudo(t)
		writeControlFake(t, dir, "code", "1")

		err := ControlInstance(context.Background(), InstanceOperationUpdate, "Minecraft01")
		if err == nil {
			t.Fatal("esperava erro")
		}

		if !strings.Contains(err.Error(), "exit status 1") {
			t.Fatalf("erro %q deveria citar o código de saída", err)
		}

		if strings.HasSuffix(strings.TrimSpace(err.Error()), ":") {
			t.Fatalf("erro %q não deveria terminar com saída vazia", err)
		}
	})

	t.Run("saída só com sequências também usa mensagem padrão", func(t *testing.T) {
		dir := installFakeControlSudo(t)
		writeControlFake(t, dir, "code", "1")
		writeControlFake(t, dir, "out", "\x1b[0m\n")

		err := ControlInstance(context.Background(), InstanceOperationStop, "Minecraft01")
		if err == nil {
			t.Fatal("esperava erro")
		}

		if strings.Contains(err.Error(), "\x1b") {
			t.Fatalf("erro %q não deveria conter ESC", err)
		}
	})

	t.Run("executável inexistente", func(t *testing.T) {
		installFakeControlSudo(t)

		config := currentRuntimeConfig()
		config.SudoPath = "/caminho/que/nao/existe/sudo"

		if err := ConfigureRuntime(config); err != nil {
			t.Fatalf("ConfigureRuntime retornou erro: %v", err)
		}

		if err := ControlInstance(context.Background(), InstanceOperationStart, "Minecraft01"); err == nil {
			t.Fatal("esperava erro ao executar sudo inexistente")
		}
	})
}

func TestControlInstanceContext(t *testing.T) {
	t.Run("tempo limite", func(t *testing.T) {
		dir := installFakeControlSudo(t)
		writeControlFake(t, dir, "sleep", "")

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := ControlInstance(ctx, InstanceOperationUpdate, "Minecraft01")
		if err == nil {
			t.Fatal("esperava erro de tempo limite")
		}

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("o erro deveria encapsular DeadlineExceeded: %v", err)
		}

		if !strings.Contains(err.Error(), "update") || !strings.Contains(err.Error(), "Minecraft01") {
			t.Fatalf("erro %q deveria citar operação e instância", err)
		}
	})

	t.Run("cancelamento", func(t *testing.T) {
		dir := installFakeControlSudo(t)
		writeControlFake(t, dir, "sleep", "")

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)

		err := ControlInstance(ctx, InstanceOperationStop, "Minecraft01")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("o erro deveria encapsular Canceled: %v", err)
		}
	})

	t.Run("contexto já cancelado não deixa sucesso passar", func(t *testing.T) {
		installFakeControlSudo(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		if err := ControlInstance(ctx, InstanceOperationStart, "Minecraft01"); err == nil {
			t.Fatal("esperava erro com contexto cancelado")
		}
	})
}
