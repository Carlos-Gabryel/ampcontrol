package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DiscordToken                 string
	DiscordNotificationChannelID string
	DiscordNotificationTTL       time.Duration
	DiscordStatusRefreshInterval time.Duration
	AMPUsername                  string
	AMPPassword                  string
	AMPADSURL                    string
	LogLevel                     string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DiscordToken: strings.TrimSpace(
			os.Getenv("DISCORD_TOKEN"),
		),
		DiscordNotificationChannelID: strings.TrimSpace(
			os.Getenv("DISCORD_NOTIFICATION_CHANNEL_ID"),
		),
		AMPUsername: strings.TrimSpace(
			os.Getenv("AMP_USERNAME"),
		),
		AMPPassword: os.Getenv("AMP_PASSWORD"),
		AMPADSURL: strings.TrimSpace(
			os.Getenv("AMP_ADS_URL"),
		),
		LogLevel: strings.TrimSpace(
			os.Getenv("LOG_LEVEL"),
		),
	}

	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf(
			"DISCORD_TOKEN não foi configurado",
		)
	}

	if cfg.DiscordNotificationChannelID == "" {
		return nil, fmt.Errorf(
			"DISCORD_NOTIFICATION_CHANNEL_ID não foi configurado",
		)
	}

	_, err := strconv.ParseUint(
		cfg.DiscordNotificationChannelID,
		10,
		64,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"DISCORD_NOTIFICATION_CHANNEL_ID é inválido: %w",
			err,
		)
	}

	if cfg.AMPUsername == "" {
		return nil, fmt.Errorf(
			"AMP_USERNAME não foi configurado",
		)
	}

	if cfg.AMPPassword == "" {
		return nil, fmt.Errorf(
			"AMP_PASSWORD não foi configurado",
		)
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	if cfg.AMPADSURL == "" {
		cfg.AMPADSURL = "http://127.0.0.1:8080"
	}

	notificationTTLMinutes, err := positiveEnvironmentInteger(
		"DISCORD_NOTIFICATION_TTL_MINUTES",
		10,
	)
	if err != nil {
		return nil, err
	}

	statusRefreshSeconds, err := positiveEnvironmentInteger(
		"DISCORD_STATUS_REFRESH_SECONDS",
		60,
	)
	if err != nil {
		return nil, err
	}

	cfg.DiscordNotificationTTL = time.Duration(notificationTTLMinutes) * time.Minute
	cfg.DiscordStatusRefreshInterval = time.Duration(statusRefreshSeconds) * time.Second

	return cfg, nil
}

func positiveEnvironmentInteger(
	name string,
	defaultValue int,
) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf(
			"%s precisa ser um número inteiro maior que zero",
			name,
		)
	}

	return value, nil
}
