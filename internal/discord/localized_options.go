package discord

import disgoDiscord "github.com/disgoorg/disgo/discord"

func optStringAny(data disgoDiscord.SlashCommandInteractionData, names ...string) (string, bool) {
	for _, name := range names {
		if value, ok := data.OptString(name); ok {
			return value, true
		}
	}
	return "", false
}

func optIntAny(data disgoDiscord.SlashCommandInteractionData, names ...string) (int, bool) {
	for _, name := range names {
		if value, ok := data.OptInt(name); ok {
			return value, true
		}
	}
	return 0, false
}
