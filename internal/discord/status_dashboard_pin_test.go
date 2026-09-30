package discord

import (
	"net/http"
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
	gets     int
}

func discordNotFound() error {
	return &rest.Error{Response: &http.Response{StatusCode: http.StatusNotFound}}
}

func (f *fakeDashboardChannels) GetMessage(_ snowflake.ID, messageID snowflake.ID, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	f.gets++
	message, ok := f.messages[messageID]
	if !ok {
		return nil, discordNotFound()
	}
	copied := *message
	return &copied, nil
}

func (f *fakeDashboardChannels) UpdateMessage(_ snowflake.ID, messageID snowflake.ID, update disgoDiscord.MessageUpdate, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	message, ok := f.messages[messageID]
	if !ok {
		return nil, discordNotFound()
	}
	f.updates++
	if update.Attachments != nil {
		message.Attachments = fakeAttachments(update.Files)
	}
	copied := *message
	return &copied, nil
}

func (f *fakeDashboardChannels) CreateMessage(_ snowflake.ID, create disgoDiscord.MessageCreate, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	f.nextID++
	message := &disgoDiscord.Message{ID: f.nextID, Flags: disgoDiscord.MessageFlagIsComponentsV2, Attachments: fakeAttachments(create.Files)}
	f.messages[message.ID] = message
	copied := *message
	return &copied, nil
}

func fakeAttachments(files []*disgoDiscord.File) []disgoDiscord.Attachment {
	attachments := make([]disgoDiscord.Attachment, 0, len(files))
	for _, file := range files {
		attachments = append(attachments, disgoDiscord.Attachment{Filename: file.Name})
	}
	return attachments
}

func (f *fakeDashboardChannels) DeleteMessage(_ snowflake.ID, messageID snowflake.ID, _ ...rest.RequestOpt) error {
	if _, ok := f.messages[messageID]; !ok {
		return discordNotFound()
	}
	delete(f.messages, messageID)
	return nil
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

func TestStatusDashboardVerifiesEachMessageOnlyOnce(t *testing.T) {
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

	if channels.gets != 1 {
		t.Fatalf("a mensagem do painel deveria ser consultada só na primeira atualização: %d consultas", channels.gets)
	}
	if channels.updates != 3 {
		t.Fatalf("o painel deveria ser editado a cada atualização: %d edições", channels.updates)
	}
}

func TestStatusDashboardRecreatesMessageDeletedAfterVerification(t *testing.T) {
	channels := &fakeDashboardChannels{
		messages: map[snowflake.ID]*disgoDiscord.Message{
			500: {ID: 500, Pinned: true, Flags: disgoDiscord.MessageFlagIsComponentsV2},
		},
		nextID: 1000,
	}
	client := newDashboardPinTestClient(t, channels, "500")
	pages, statuses := dashboardPinTestPages()

	if err := client.upsertStatusDashboardMessages(pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	delete(channels.messages, 500)
	if err := client.upsertStatusDashboardMessages(pages, statuses); err != nil {
		t.Fatalf("painel apagado deveria ser recriado sem erro: %v", err)
	}

	state, err := loadStatusDashboardState(client.statusStatePath)
	if err != nil {
		t.Fatalf("loadStatusDashboardState retornou erro: %v", err)
	}
	if len(state.MessageIDs) != 1 || state.MessageIDs[0] != "1001" {
		t.Fatalf("o estado deveria apontar para o painel recriado: %+v", state)
	}
	if !channels.messages[1001].Pinned {
		t.Fatal("o painel recriado deveria ser fixado")
	}
}

func TestCommandGuideSkipsUnchangedEdits(t *testing.T) {
	channels := &fakeDashboardChannels{
		messages: map[snowflake.ID]*disgoDiscord.Message{
			700: {ID: 700, Pinned: true},
		},
		nextID: 1000,
	}
	client := newDashboardPinTestClient(t, channels)
	if err := saveStatusDashboardState(client.statusStatePath, statusDashboardState{GuideMessageID: "700"}); err != nil {
		t.Fatalf("não foi possível preparar o estado do guia: %v", err)
	}

	for range 3 {
		if err := client.upsertCommandGuideMessage(); err != nil {
			t.Fatalf("upsertCommandGuideMessage retornou erro: %v", err)
		}
	}
	if channels.updates != 1 {
		t.Fatalf("o guia sem mudanças deveria ser editado só uma vez: %d edições", channels.updates)
	}

	delete(channels.messages, 700)
	if err := client.upsertCommandGuideMessage(); err != nil {
		t.Fatalf("upsertCommandGuideMessage retornou erro: %v", err)
	}
	if _, recreated := channels.messages[1001]; !recreated {
		t.Fatal("guia apagado deveria ser recriado")
	}
}

func TestStatusDashboardSwapsLogoOfVerifiedMessageWithoutQuerying(t *testing.T) {
	channels := &fakeDashboardChannels{messages: map[snowflake.ID]*disgoDiscord.Message{}, nextID: 1000}
	client := newDashboardPinTestClient(t, channels)
	refresh := func(game string) {
		t.Helper()
		statuses := []ampInstanceStatusView{{Instance: amp.ManagedInstance{Name: "Jogo01", Game: game}}}
		pages := buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22")
		if err := client.upsertStatusDashboardMessages(pages, statuses); err != nil {
			t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
		}
	}

	refresh("Minecraft")
	refresh("Valheim")
	refresh("Valheim")

	attachments := channels.messages[1001].Attachments
	if len(attachments) != 1 || attachments[0].Filename != "valheim.png" {
		t.Fatalf("o painel deveria trocar para o logo do novo jogo: %+v", attachments)
	}
	if channels.gets != 0 {
		t.Fatalf("mensagem criada pelo próprio bot não precisa ser consultada: %d consultas", channels.gets)
	}
	if len(channels.messages) != 1 {
		t.Fatalf("a troca de logo não deveria recriar a mensagem: %d mensagens", len(channels.messages))
	}
}
