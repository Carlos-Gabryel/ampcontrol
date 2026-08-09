package app

import (
	"context"
	"fmt"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/alabamaamp/ampcontrol/internal/config"
	discordClient "github.com/alabamaamp/ampcontrol/internal/discord"
	"github.com/alabamaamp/ampcontrol/internal/idle"
	"github.com/alabamaamp/ampcontrol/internal/logger"
	"github.com/alabamaamp/ampcontrol/internal/operation"
)

type App struct {
	Discord      *discordClient.Client
	IdleObserver *idleObserver
	Operations   *operation.Manager
}

func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	log := logger.New(
		cfg.LogLevel,
	)

	ampAPIClient := amp.NewAPIClient(
		cfg.AMPUsername,
		cfg.AMPPassword,
	)

	operationManager := operation.NewManager()

	idleConfig, err := idle.Load(
		idleObserverConfigPath,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"não foi possível carregar a configuração compartilhada do Idle: %w",
			err,
		)
	}

	discord, err := discordClient.New(
		cfg.DiscordToken,
		ampAPIClient,
		cfg.DiscordNotificationChannelID,
		log,
	)
	if err != nil {
		return nil, err
	}

	if err := discord.SetOperationManager(
		operationManager,
	); err != nil {
		return nil, err
	}

	idleObserver, err := newIdleObserver(
		ampAPIClient,
		operationManager,
		discord,
		idleConfig,
		log,
	)
	if err != nil {
		return nil, err
	}

	discord.EnableAMPCommandHandling()

	return &App{
		Discord:      discord,
		IdleObserver: idleObserver,
		Operations:   operationManager,
	}, nil
}

func (a *App) Start(
	ctx context.Context,
) error {
	if err := a.Discord.Start(ctx); err != nil {
		return err
	}

	if a.IdleObserver != nil {
		go a.IdleObserver.Run(
			ctx,
		)
	}

	return nil
}
