package discord

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
	"github.com/rs/zerolog"
)

// fakeDashboardChannels simula só as rotas do Discord usadas pelo painel.
type fakeDashboardChannels struct {
	rest.Channels
	messages map[snowflake.ID]*disgoDiscord.Message
	nextID   snowflake.ID
	pins     int
	updates  int
}

func (f *fakeDashboardChannels) GetMessage(_ snowflake.ID, messageID snowflake.ID, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	message, ok := f.messages[messageID]
	if !ok {
		return nil, &rest.Error{}
	}
	copied := *message
	return &copied, nil
}

func (f *fakeDashboardChannels) UpdateMessage(_ snowflake.ID, messageID snowflake.ID, _ disgoDiscord.MessageUpdate, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	f.updates++
	copied := *f.messages[messageID]
	return &copied, nil
}

func (f *fakeDashboardChannels) CreateMessage(_ snowflake.ID, _ disgoDiscord.MessageCreate, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	f.nextID++
	message := &disgoDiscord.Message{ID: f.nextID, Flags: disgoDiscord.MessageFlagIsComponentsV2}
	f.messages[message.ID] = message
	copied := *message
	return &copied, nil
}

func (f *fakeDashboardChannels) PinMessage(_ snowflake.ID, messageID snowflake.ID, _ ...rest.RequestOpt) error {
	f.pins++
	f.messages[messageID].Pinned = true
	return nil
}

func newDashboardPinTestClient(t *testing.T, channels *fakeDashboardChannels, messageIDs ...string) *Client {
	t.Helper()
	statePath := filepath.Join(t.TempDir(), "discord_status.json")
	if err := saveStatusDashboardState(statePath, statusDashboardState{MessageIDs: messageIDs}); err != nil {
		t.Fatalf("não foi possível preparar o estado do painel: %v", err)
	}
	return &Client{
		bot:                   &bot.Client{Caches: cache.New()},
		channels:              channels,
		notificationChannelID: 1,
		statusStatePath:       statePath,
		log:                   zerolog.Nop(),
	}
}

func dashboardPinTestPages() ([][]disgoDiscord.LayoutComponent, []ampInstanceStatusView) {
	statuses := []ampInstanceStatusView{{Instance: amp.ManagedInstance{Name: "Jogo01", Game: "Jogo sem logo"}}}
	return buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22"), statuses
}

func TestStatusDashboardDoesNotRepinPinnedMessage(t *testing.T) {
	channels := &fakeDashboardChannels{
		messages: map[snowflake.ID]*disgoDiscord.Message{
			500: {ID: 500, Pinned: true, Flags: disgoDiscord.MessageFlagIsComponentsV2},
		},
		nextID: 1000,
	}
	client := newDashboardPinTestClient(t, channels, "500")
	pages, statuses := dashboardPinTestPages()

	for range 3 {
		if err := client.upsertStatusDashboardMessages(pages, statuses); err != nil {
			t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
		}
	}

	if channels.updates != 3 {
		t.Fatalf("o painel deveria ser editado a cada atualização: %d edições", channels.updates)
	}
	if channels.pins != 0 {
		t.Fatalf("mensagem já fixada não deveria ser fixada de novo: %d chamadas de pin", channels.pins)
	}
}

func TestStatusDashboardPinsUnpinnedAndNewMessages(t *testing.T) {
	channels := &fakeDashboardChannels{
		messages: map[snowflake.ID]*disgoDiscord.Message{
			500: {ID: 500, Flags: disgoDiscord.MessageFlagIsComponentsV2},
		},
		nextID: 1000,
	}
	client := newDashboardPinTestClient(t, channels, "500")
	pages, statuses := dashboardPinTestPages()

	if err := client.upsertStatusDashboardMessages(pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	if channels.pins != 1 || !channels.messages[500].Pinned {
		t.Fatalf("mensagem existente sem pin deveria ser fixada uma vez: %d chamadas", channels.pins)
	}

	channels.pins = 0
	fresh := newDashboardPinTestClient(t, channels)
	if err := fresh.upsertStatusDashboardMessages(pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	if channels.pins != 1 || !channels.messages[1001].Pinned {
		t.Fatalf("mensagem nova deveria ser fixada uma vez: %d chamadas", channels.pins)
	}
}
