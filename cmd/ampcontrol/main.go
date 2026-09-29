package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/app"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
)

var version = "dev"

// shutdownTimeout cobre a operação manual mais longa (14 minutos) com folga;
// a unidade systemd usa TimeoutStopSec maior que este valor.
const shutdownTimeout = 15 * time.Minute

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "--version":
			fmt.Printf("AmpControl %s\n", version)
			return
		case "--check-config":
			if _, err := app.New(); err != nil {
				fmt.Println(i18n.T(i18n.ConfigInvalid))
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println(i18n.T(i18n.ConfigValid))
			return
		}
	}

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

	defer cancel()

	application, err := app.New()

	if err != nil {
		fmt.Println(i18n.T(i18n.StartupError))
		fmt.Println(err)
		os.Exit(1)
	}

	err = application.Start(ctx)

	if err != nil {
		fmt.Println(i18n.T(i18n.DiscordError))
		fmt.Println(err)
		os.Exit(1)
	}

	<-ctx.Done()
	// Devolve o comportamento padrão dos sinais: um segundo Ctrl+C encerra
	// na hora, sem esperar as operações.
	cancel()

	fmt.Println(i18n.T(i18n.ShutdownWaiting))
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer shutdownCancel()

	if remaining, err := application.Shutdown(shutdownCtx); err != nil {
		fmt.Println(i18n.T(i18n.ShutdownTimeout))
		for _, info := range remaining {
			fmt.Printf("- %s: %s\n", info.Instance, info.Operation)
		}
	}
}
