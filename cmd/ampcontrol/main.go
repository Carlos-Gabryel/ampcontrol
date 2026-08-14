package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Carlos-Gabryel/ampcontrol/internal/app"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 {
		switch os.Args[1] {
		case "--version":
			fmt.Printf("AmpControl %s\n", version)
			return
		case "--check-config":
			if _, err := app.New(); err != nil {
				fmt.Println("Configuração inválida:")
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Println("Configuração válida.")
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
		fmt.Println("Erro ao iniciar AmpControl:")
		fmt.Println(err)
		os.Exit(1)
	}

	err = application.Start(ctx)

	if err != nil {
		fmt.Println("Erro Discord:")
		fmt.Println(err)
		os.Exit(1)
	}

	<-ctx.Done()
}
