package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	"github.com/Carlos-Gabryel/ampcontrol/internal/secret"
	"github.com/joho/godotenv"
)

type Config struct {
	Language                      i18n.Language
	DiscordToken                  string
	DiscordGuildID                string
	DiscordNotificationChannelID  string
	DiscordAuditChannelID         string
	DiscordOwnerUserID            string
	DiscordAdminRoleIDs           []string
	DiscordRestrictCommandChannel bool
	DiscordAllowAdministrators    bool
	DiscordNotificationTTL        time.Duration
	DiscordStatusRefreshInterval  time.Duration
	DiscordCommandUserCooldown    time.Duration
	DiscordCommandServerCooldown  time.Duration
	AMPUsername                   string
	AMPPassword                   string
	AMPADSURL                     string
	AMPPublicURL                  string
	AMPGameServerAddress          string
	AMPSystemUser                 string
	AMPManagerPath                string
	AMPWrapperPath                string
	SudoPath                      string
	LogLevel                      string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	fileConfig, err := loadFileConfig()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Language: i18n.Language(strings.TrimSpace(os.Getenv("AMPCONTROL_LANGUAGE"))),
		DiscordGuildID: strings.TrimSpace(
			os.Getenv("DISCORD_GUILD_ID"),
		),
		DiscordNotificationChannelID: strings.TrimSpace(
			os.Getenv("DISCORD_NOTIFICATION_CHANNEL_ID"),
		),
		DiscordAuditChannelID: strings.TrimSpace(
			os.Getenv("DISCORD_AUDIT_CHANNEL_ID"),
		),
		DiscordOwnerUserID: strings.TrimSpace(
			os.Getenv("DISCORD_OWNER_USER_ID"),
		),
		AMPUsername: strings.TrimSpace(
			os.Getenv("AMP_USERNAME"),
		),
		AMPADSURL: strings.TrimSpace(
			os.Getenv("AMP_ADS_URL"),
		),
		AMPPublicURL: strings.TrimRight(strings.TrimSpace(
			os.Getenv("AMP_PUBLIC_URL"),
		), "/"),
		AMPGameServerAddress: strings.TrimSpace(
			os.Getenv("AMP_GAME_SERVER_ADDRESS"),
		),
		LogLevel: strings.TrimSpace(
			os.Getenv("LOG_LEVEL"),
		),
	}
	applyFileDefaults(cfg, fileConfig)
	cfg.Language, err = i18n.Parse(string(cfg.Language))
	if err != nil {
		return nil, err
	}

	cfg.DiscordToken, err = secret.ReadRequired("discord_token", "DISCORD_TOKEN")
	if err != nil {
		return nil, err
	}
	cfg.AMPPassword, err = secret.ReadRequired("amp_password", "AMP_PASSWORD")
	if err != nil {
		return nil, err
	}

	if cfg.DiscordToken == "" {
		return nil, fmt.Errorf(
			"DISCORD_TOKEN não foi configurado",
		)
	}

	if err := validateDiscordID("DISCORD_GUILD_ID", cfg.DiscordGuildID); err != nil {
		return nil, err
	}

	for name, value := range map[string]string{
		"DISCORD_NOTIFICATION_CHANNEL_ID": cfg.DiscordNotificationChannelID,
		"DISCORD_AUDIT_CHANNEL_ID":        cfg.DiscordAuditChannelID,
		"DISCORD_OWNER_USER_ID":           cfg.DiscordOwnerUserID,
	} {
		if err := validateDiscordID(name, value); err != nil {
			return nil, err
		}
	}
	for _, roleID := range cfg.DiscordAdminRoleIDs {
		if err := validateDiscordID("discord.admin_role_ids", roleID); err != nil {
			return nil, err
		}
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
		valueOrDefault(fileConfig.Discord.NotificationTTLMinutes, 10),
	)
	if err != nil {
		return nil, err
	}

	statusRefreshSeconds, err := positiveEnvironmentInteger(
		"DISCORD_STATUS_REFRESH_SECONDS",
		valueOrDefault(fileConfig.Discord.StatusRefreshSeconds, 60),
	)
	if err != nil {
		return nil, err
	}

	userCooldownSeconds, err := nonNegativeEnvironmentInteger(
		"DISCORD_COMMAND_USER_COOLDOWN_SECONDS",
		valueOrDefault(fileConfig.Discord.CommandUserCooldownSeconds, 5),
	)
	if err != nil {
		return nil, err
	}

	serverCooldownSeconds, err := nonNegativeEnvironmentInteger(
		"DISCORD_COMMAND_SERVER_COOLDOWN_SECONDS",
		valueOrDefault(fileConfig.Discord.CommandServerCooldownSeconds, 15),
	)
	if err != nil {
		return nil, err
	}

	cfg.DiscordNotificationTTL = time.Duration(notificationTTLMinutes) * time.Minute
	cfg.DiscordStatusRefreshInterval = time.Duration(statusRefreshSeconds) * time.Second
	cfg.DiscordCommandUserCooldown = time.Duration(userCooldownSeconds) * time.Second
	cfg.DiscordCommandServerCooldown = time.Duration(serverCooldownSeconds) * time.Second

	return cfg, nil
}

func validateDiscordID(name string, value string) error {
	if value == "" {
		return fmt.Errorf("%s não foi configurado", name)
	}

	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil || id == 0 {
		return fmt.Errorf("%s é inválido", name)
	}

	return nil
}

func nonNegativeEnvironmentInteger(
	name string,
	defaultValue int,
) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf(
			"%s precisa ser um número inteiro maior ou igual a zero",
			name,
		)
	}

	return value, nil
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
