package app

import (
	"context"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/config"
	discordClient "github.com/alabamaamp/palcontrol/internal/discord"
	"github.com/alabamaamp/palcontrol/internal/logger"
	"github.com/alabamaamp/palcontrol/internal/operation"
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

	discord, err := discordClient.New(
		cfg.DiscordToken,
		ampAPIClient,
		cfg.AlamamaRCONPassword,
		cfg.KalagaRCONPassword,
		cfg.DiscordNotificationChannelID,
		cfg.IdleTimeout,
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
