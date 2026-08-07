package app

import (
	"context"

	"github.com/alabamaamp/palcontrol/internal/amp"
	"github.com/alabamaamp/palcontrol/internal/config"
	discordClient "github.com/alabamaamp/palcontrol/internal/discord"
	"github.com/alabamaamp/palcontrol/internal/logger"
)

type App struct {
	Discord *discordClient.Client
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

	discord.EnableAMPCommandHandling()

	return &App{
		Discord: discord,
	}, nil
}

func (a *App) Start(
	ctx context.Context,
) error {
	return a.Discord.Start(ctx)
}
