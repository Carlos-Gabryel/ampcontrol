package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadFileConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := []byte(`language = "en-US"

[discord]
guild_id = "111111111111111111"
notification_channel_id = "222222222222222222"
audit_channel_id = "333333333333333333"
owner_user_id = "444444444444444444"
admin_role_ids = ["555555555555555555"]
restrict_commands_to_channel = false
allow_discord_administrators = false
status_refresh_seconds = 30

[amp]
username = "amp"
ads_url = "http://127.0.0.1:8080"

[logging]
level = "debug"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AMPCONTROL_CONFIG", path)

	result, err := loadFileConfig()
	if err != nil {
		t.Fatal(err)
	}
	if result.Discord.GuildID != "111111111111111111" || result.AMP.Username != "amp" {
		t.Fatalf("configuração inesperada: %#v", result)
	}
	if result.Language != "en-US" {
		t.Fatalf("idioma inesperado: %q", result.Language)
	}
	if result.Discord.RestrictCommandsToChannel == nil || *result.Discord.RestrictCommandsToChannel ||
		len(result.Discord.AdminRoleIDs) != 1 {
		t.Fatalf("política Discord inesperada: %#v", result.Discord)
	}
}

func TestLoadFileConfigRejectsUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("unknown = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AMPCONTROL_CONFIG", path)
	if _, err := loadFileConfig(); err == nil {
		t.Fatal("era esperado erro para campo desconhecido")
	}
}

func TestLoadUsesTOMLAndSystemdCredentials(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	credentialDirectory := filepath.Join(directory, "credentials")
	if err := os.Mkdir(credentialDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte(`language = "en-US"

[discord]
guild_id = "111111111111111111"
notification_channel_id = "222222222222222222"
audit_channel_id = "333333333333333333"
owner_user_id = "444444444444444444"

[amp]
username = "amp-api-user"
`)
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"discord_token": "discord-secret",
		"amp_password":  "amp-secret",
	} {
		if err := os.WriteFile(filepath.Join(credentialDirectory, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv("AMPCONTROL_CONFIG", configPath)
	t.Setenv("CREDENTIALS_DIRECTORY", credentialDirectory)
	for _, name := range []string{
		"DISCORD_TOKEN", "DISCORD_GUILD_ID", "DISCORD_NOTIFICATION_CHANNEL_ID",
		"DISCORD_AUDIT_CHANNEL_ID", "DISCORD_OWNER_USER_ID", "AMP_USERNAME", "AMP_PASSWORD",
	} {
		t.Setenv(name, "")
	}

	result, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if result.DiscordToken != "discord-secret" || result.AMPPassword != "amp-secret" {
		t.Fatal("as credenciais do systemd não foram carregadas")
	}
	if result.DiscordGuildID != "111111111111111111" || result.AMPUsername != "amp-api-user" {
		t.Fatalf("configuração TOML inesperada: %#v", result)
	}
	if result.Language != "en-US" {
		t.Fatalf("idioma inesperado: %q", result.Language)
	}
}

func TestBilingualExampleConfigsParse(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	for filename, expectedLanguage := range map[string]string{
		"ampcontrol.example.toml":    "pt-BR",
		"ampcontrol.example.en.toml": "en-US",
	} {
		t.Run(filename, func(t *testing.T) {
			t.Setenv("AMPCONTROL_CONFIG", filepath.Join(root, "config", filename))
			result, err := loadFileConfig()
			if err != nil {
				t.Fatal(err)
			}
			if result.Language != expectedLanguage {
				t.Fatalf("language = %q, want %q", result.Language, expectedLanguage)
			}
		})
	}
}
