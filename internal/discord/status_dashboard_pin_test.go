package discord

import (
	"context"
	"fmt"
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
	writes   []time.Time
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
	f.writes = append(f.writes, time.Now())
	if update.Attachments != nil {
		message.Attachments = fakeAttachments(update.Files)
	}
	copied := *message
	return &copied, nil
}

func (f *fakeDashboardChannels) CreateMessage(_ snowflake.ID, create disgoDiscord.MessageCreate, _ ...rest.RequestOpt) (*disgoDiscord.Message, error) {
	f.nextID++
	f.writes = append(f.writes, time.Now())
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

// forceDashboardEditEachRefresh faz cada atualização cair numa edição
// forçada, para testes que precisam de uma edição por atualização mesmo
// com o mesmo conteúdo.
func forceDashboardEditEachRefresh(client *Client) {
	now := time.Date(2026, time.September, 30, 2, 0, 0, 0, time.UTC)
	client.dashboardNow = func() time.Time {
		now = now.Add(statusDashboardForcedEditInterval)
		return now
	}
}

func TestStatusDashboardDoesNotRepinPinnedMessage(t *testing.T) {
	channels := &fakeDashboardChannels{
		messages: map[snowflake.ID]*disgoDiscord.Message{
			500: {ID: 500, Pinned: true, Flags: disgoDiscord.MessageFlagIsComponentsV2},
		},
		nextID: 1000,
	}
	client := newDashboardPinTestClient(t, channels, "500")
	forceDashboardEditEachRefresh(client)
	pages, statuses := dashboardPinTestPages()

	for range 3 {
		if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
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

	if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	if channels.pins != 1 || !channels.messages[500].Pinned {
		t.Fatalf("mensagem existente sem pin deveria ser fixada uma vez: %d chamadas", channels.pins)
	}

	channels.pins = 0
	fresh := newDashboardPinTestClient(t, channels)
	if err := fresh.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
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
	forceDashboardEditEachRefresh(client)
	pages, statuses := dashboardPinTestPages()

	for range 3 {
		if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
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
	forceDashboardEditEachRefresh(client)
	pages, statuses := dashboardPinTestPages()

	if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	delete(channels.messages, 500)
	if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
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
		if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
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

func dashboardPacingTestPages(count int) ([][]disgoDiscord.LayoutComponent, []ampInstanceStatusView) {
	statuses := make([]ampInstanceStatusView, count)
	for index := range statuses {
		statuses[index].Instance = amp.ManagedInstance{Name: fmt.Sprintf("Jogo%02d", index), Game: "Jogo sem logo"}
	}
	return buildAMPStatusPages(statuses, time.Unix(1_700_000_000, 0), "192.168.1.22"), statuses
}

func TestStatusDashboardSpacesMessageWrites(t *testing.T) {
	const spacing = 40 * time.Millisecond
	channels := &fakeDashboardChannels{messages: map[snowflake.ID]*disgoDiscord.Message{}, nextID: 1000}
	client := newDashboardPinTestClient(t, channels)
	client.dashboardEditSpacing = spacing
	pages, statuses := dashboardPacingTestPages(4)

	if err := client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}

	if len(channels.writes) != 4 {
		t.Fatalf("esperadas 4 escritas no Discord: %d", len(channels.writes))
	}
	for index := 1; index < len(channels.writes); index++ {
		if gap := channels.writes[index].Sub(channels.writes[index-1]); gap < spacing {
			t.Fatalf("escritas %d e %d com intervalo de %v; mínimo %v", index-1, index, gap, spacing)
		}
	}
}

func TestStatusDashboardFinishesWithoutSpacingAfterDeadline(t *testing.T) {
	channels := &fakeDashboardChannels{messages: map[snowflake.ID]*disgoDiscord.Message{}, nextID: 1000}
	client := newDashboardPinTestClient(t, channels)
	client.dashboardEditSpacing = time.Hour
	pages, statuses := dashboardPacingTestPages(3)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	if err := client.upsertStatusDashboardMessages(ctx, pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("com o prazo esgotado o painel não deveria esperar: %v", elapsed)
	}

	state, err := loadStatusDashboardState(client.statusStatePath)
	if err != nil {
		t.Fatalf("loadStatusDashboardState retornou erro: %v", err)
	}
	if len(state.MessageIDs) != 3 {
		t.Fatalf("todas as mensagens criadas precisam ficar registradas no estado: %+v", state)
	}
}

func TestStatusDashboardEditSpacingFitsBudget(t *testing.T) {
	tests := []struct {
		pages    int
		expected time.Duration
	}{
		{pages: 0, expected: 1500 * time.Millisecond},
		{pages: 1, expected: 1500 * time.Millisecond},
		{pages: 12, expected: 1500 * time.Millisecond},
		{pages: 40, expected: 500 * time.Millisecond},
	}
	for _, test := range tests {
		actual := dashboardEditSpacing(1500*time.Millisecond, 20*time.Second, test.pages)
		if actual != test.expected {
			t.Fatalf("espaçamento para %d páginas: %v; esperado %v", test.pages, actual, test.expected)
		}
	}
}

type dashboardSkipTest struct {
	channels *fakeDashboardChannels
	client   *Client
	now      time.Time
}

func newDashboardSkipTest(t *testing.T) *dashboardSkipTest {
	test := &dashboardSkipTest{
		channels: &fakeDashboardChannels{messages: map[snowflake.ID]*disgoDiscord.Message{}, nextID: 1000},
		now:      time.Date(2026, time.September, 30, 2, 0, 0, 0, time.UTC),
	}
	test.client = newDashboardPinTestClient(t, test.channels)
	test.client.dashboardNow = func() time.Time { return test.now }
	return test
}

func (d *dashboardSkipTest) refresh(t *testing.T, names ...string) {
	t.Helper()
	statuses := make([]ampInstanceStatusView, len(names))
	for index, name := range names {
		statuses[index].Instance = amp.ManagedInstance{Name: fmt.Sprintf("Jogo%02d", index), FriendlyName: name, Game: "Jogo sem logo"}
	}
	// O rodapé muda a cada atualização, como em produção.
	pages := buildAMPStatusPages(statuses, d.now, "192.168.1.22")
	if err := d.client.upsertStatusDashboardMessages(context.Background(), pages, statuses); err != nil {
		t.Fatalf("upsertStatusDashboardMessages retornou erro: %v", err)
	}
}

func TestStatusDashboardSkipsUnchangedCards(t *testing.T) {
	test := newDashboardSkipTest(t)
	test.refresh(t, "Alfa", "Beta", "Gama")
	writes := len(test.channels.writes)
	// Passos menores que a primeira edição forçada (intervalo/3 cards).

	for range 3 {
		test.now = test.now.Add(20 * time.Second)
		test.refresh(t, "Alfa", "Beta", "Gama")
	}
	if extra := len(test.channels.writes) - writes; extra != 0 {
		t.Fatalf("cards sem mudança não deveriam ser editados: %d escritas extras", extra)
	}
	if test.channels.gets != 0 {
		t.Fatalf("cards sem mudança não deveriam ser consultados: %d consultas", test.channels.gets)
	}

	test.now = test.now.Add(20 * time.Second)
	test.refresh(t, "Alfa", "Beta mudou", "Gama")
	if extra := len(test.channels.writes) - writes; extra != 1 {
		t.Fatalf("só o card que mudou deveria ser editado: %d escritas", extra)
	}
}

func TestStatusDashboardStaggersForcedEdits(t *testing.T) {
	test := newDashboardSkipTest(t)
	names := []string{"Alfa", "Beta", "Gama", "Delta", "Épsilon"}
	test.refresh(t, names...)
	writes := len(test.channels.writes)

	// Sem mudanças, cada card precisa ser editado uma vez a cada intervalo
	// forçado, mas sem que todos vençam no mesmo ciclo.
	for minute := 1; minute <= 5; minute++ {
		test.now = test.now.Add(time.Minute)
		test.refresh(t, names...)
		if perMinute := len(test.channels.writes) - writes; perMinute > 1 {
			t.Fatalf("minuto %d: %d edições forçadas no mesmo ciclo", minute, perMinute)
		}
		writes = len(test.channels.writes)
	}
	if test.channels.updates != len(names) {
		t.Fatalf("cada card deveria ser editado uma vez em %v: %d edições", statusDashboardForcedEditInterval, test.channels.updates)
	}

	// Depois do primeiro ciclo cada card mantém o próprio ritmo de 5 min.
	for range 5 {
		test.now = test.now.Add(time.Minute)
		test.refresh(t, names...)
	}
	if test.channels.updates != 2*len(names) {
		t.Fatalf("no intervalo seguinte cada card deveria ser editado mais uma vez: %d edições", test.channels.updates)
	}
}

func TestStatusDashboardRecreatesDeletedCardOnForcedEdit(t *testing.T) {
	test := newDashboardSkipTest(t)
	test.refresh(t, "Alfa")
	delete(test.channels.messages, 1001)

	test.now = test.now.Add(statusDashboardForcedEditInterval)
	test.refresh(t, "Alfa")
	if message, ok := test.channels.messages[1002]; !ok || !message.Pinned {
		t.Fatalf("card apagado deveria ser recriado e fixado na edição forçada: %+v", test.channels.messages)
	}
}
