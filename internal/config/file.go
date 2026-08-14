package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const defaultConfigPath = "/etc/ampcontrol/config.toml"

type fileConfig struct {
	Discord fileDiscordConfig `toml:"discord"`
	AMP     fileAMPConfig     `toml:"amp"`
	Logging fileLoggingConfig `toml:"logging"`
}

type fileDiscordConfig struct {
	GuildID                      string   `toml:"guild_id"`
	NotificationChannelID        string   `toml:"notification_channel_id"`
	AuditChannelID               string   `toml:"audit_channel_id"`
	OwnerUserID                  string   `toml:"owner_user_id"`
	AdminRoleIDs                 []string `toml:"admin_role_ids"`
	RestrictCommandsToChannel    *bool    `toml:"restrict_commands_to_channel"`
	AllowDiscordAdministrators   *bool    `toml:"allow_discord_administrators"`
	NotificationTTLMinutes       *int     `toml:"notification_ttl_minutes"`
	StatusRefreshSeconds         *int     `toml:"status_refresh_seconds"`
	CommandUserCooldownSeconds   *int     `toml:"command_user_cooldown_seconds"`
	CommandServerCooldownSeconds *int     `toml:"command_server_cooldown_seconds"`
}

type fileAMPConfig struct {
	Username          string `toml:"username"`
	ADSURL            string `toml:"ads_url"`
	PublicURL         string `toml:"public_url"`
	GameServerAddress string `toml:"game_server_address"`
	SystemUser        string `toml:"system_user"`
	ManagerPath       string `toml:"manager_path"`
	WrapperPath       string `toml:"wrapper_path"`
	SudoPath          string `toml:"sudo_path"`
}

type fileLoggingConfig struct {
	Level string `toml:"level"`
}

func loadFileConfig() (fileConfig, error) {
	explicitPath := strings.TrimSpace(os.Getenv("AMPCONTROL_CONFIG"))
	path := explicitPath
	if path == "" {
		path = defaultConfigPath
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) && explicitPath == "" {
			return fileConfig{}, nil
		}
		return fileConfig{}, fmt.Errorf("não foi possível abrir a configuração em %s: %w", path, err)
	}
	defer file.Close()

	var result fileConfig
	decoder := toml.NewDecoder(file).DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return fileConfig{}, fmt.Errorf("não foi possível interpretar a configuração em %s: %w", path, err)
	}
	return result, nil
}

func applyFileDefaults(cfg *Config, source fileConfig) {
	if cfg.DiscordGuildID == "" {
		cfg.DiscordGuildID = strings.TrimSpace(source.Discord.GuildID)
	}
	if cfg.DiscordNotificationChannelID == "" {
		cfg.DiscordNotificationChannelID = strings.TrimSpace(source.Discord.NotificationChannelID)
	}
	if cfg.DiscordAuditChannelID == "" {
		cfg.DiscordAuditChannelID = strings.TrimSpace(source.Discord.AuditChannelID)
	}
	if cfg.DiscordOwnerUserID == "" {
		cfg.DiscordOwnerUserID = strings.TrimSpace(source.Discord.OwnerUserID)
	}
	cfg.DiscordAdminRoleIDs = normalizeStringList(source.Discord.AdminRoleIDs)
	cfg.DiscordRestrictCommandChannel = boolOrDefault(source.Discord.RestrictCommandsToChannel, true)
	cfg.DiscordAllowAdministrators = boolOrDefault(source.Discord.AllowDiscordAdministrators, true)
	if cfg.AMPUsername == "" {
		cfg.AMPUsername = strings.TrimSpace(source.AMP.Username)
	}
	if cfg.AMPADSURL == "" {
		cfg.AMPADSURL = strings.TrimSpace(source.AMP.ADSURL)
	}
	if cfg.AMPPublicURL == "" {
		cfg.AMPPublicURL = strings.TrimRight(strings.TrimSpace(source.AMP.PublicURL), "/")
	}
	if cfg.AMPGameServerAddress == "" {
		cfg.AMPGameServerAddress = strings.TrimSpace(source.AMP.GameServerAddress)
	}
	cfg.AMPSystemUser = valueOrStringDefault(source.AMP.SystemUser, "amp")
	cfg.AMPManagerPath = valueOrStringDefault(source.AMP.ManagerPath, "/usr/bin/ampinstmgr")
	cfg.AMPWrapperPath = valueOrStringDefault(source.AMP.WrapperPath, "/usr/local/bin/ampcontrol-amp")
	cfg.SudoPath = valueOrStringDefault(source.AMP.SudoPath, "/usr/bin/sudo")
	if cfg.LogLevel == "" {
		cfg.LogLevel = strings.TrimSpace(source.Logging.Level)
	}
}

func valueOrStringDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func boolOrDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func valueOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
