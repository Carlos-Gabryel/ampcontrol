package discord

import (
	"testing"

	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	disgoDiscord "github.com/disgoorg/disgo/discord"
)

func TestEnglishCommandDefinitions(t *testing.T) {
	previous := i18n.Default()
	t.Cleanup(func() { i18n.SetDefault(previous) })
	i18n.SetDefault(i18n.EnglishUS)

	public := buildAMPCommand(nil)
	want := []string{"status", "start", "stop", "restart", "shutdown", "update"}
	for index, option := range public.Options {
		subcommand, ok := option.(disgoDiscord.ApplicationCommandOptionSubCommand)
		if !ok {
			t.Fatalf("option %d is %T", index, option)
		}
		if subcommand.Name != want[index] {
			t.Errorf("option %d = %q, want %q", index, subcommand.Name, want[index])
		}
	}

	configuration := buildAMPConfigCommand(nil, nil, nil)
	first := configuration.Options[0].(disgoDiscord.ApplicationCommandOptionSubCommand)
	if first.Name != "diagnostics" {
		t.Fatalf("first config command = %q", first.Name)
	}
	configure := configuration.Options[2].(disgoDiscord.ApplicationCommandOptionSubCommand)
	server := configure.Options[0].(disgoDiscord.ApplicationCommandOptionString)
	if configure.Name != "configure" || server.Name != "server" {
		t.Fatalf("localized configure command = %q %q", configure.Name, server.Name)
	}
}
