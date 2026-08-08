package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alabamaamp/palcontrol/internal/app"
)

func main() {
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
