package discord

import (
	"strings"
	"testing"
	"time"

	"github.com/Carlos-Gabryel/ampcontrol/internal/amp"
	"github.com/Carlos-Gabryel/ampcontrol/internal/i18n"
	disgoDiscord "github.com/disgoorg/disgo/discord"
)

func joinCodeClient(fake *fakeAMPAPI) *Client {
	return &Client{ampClient: amp.NewAPIClient("usuario", "senha")}
}

func joinCodeInstance(fake *fakeAMPAPI, game string, running bool) amp.ManagedInstance {
	return amp.ManagedInstance{Name: "Valheim01", Game: game, Running: running, APIURL: fake.server.URL}
}

func collectOne(c *Client, instance amp.ManagedInstance) ampInstanceStatusView {
	return c.collectAMPInstanceStatuses([]amp.ManagedInstance{instance})[0]
}

func TestJoinCodeFetchedOnceAndCached(t *testing.T) {
	fake := newFakeAMPAPI(t, 20)
	fake.uptime, fake.joinCode = "0:0:5:0", "454389"
	client := joinCodeClient(fake)
	instance := joinCodeInstance(fake, "Valheim", true)

	if got := collectOne(client, instance).JoinCode; got != "454389" {
		t.Fatalf("código = %q", got)
	}
	if fake.count("GetUpdates") != 1 {
		t.Fatalf("GetUpdates = %d", fake.count("GetUpdates"))
	}

	// Mesmo uptime e uptime maior: sem nova leitura do console.
	fake.mutex.Lock()
	fake.joinCode = "000000"
	fake.mutex.Unlock()
	for _, uptime := range []string{"0:0:5:0", "0:0:9:0"} {
		fake.mutex.Lock()
		fake.uptime = uptime
		fake.mutex.Unlock()
		if got := collectOne(client, instance).JoinCode; got != "454389" {
			t.Fatalf("código em cache = %q", got)
		}
	}
	if fake.count("GetUpdates") != 1 {
		t.Fatalf("GetUpdates após cache = %d", fake.count("GetUpdates"))
	}

	// Reinício: uptime menor busca o novo código.
	fake.mutex.Lock()
	fake.uptime, fake.joinCode = "0:0:1:0", "777888"
	fake.mutex.Unlock()
	if got := collectOne(client, instance).JoinCode; got != "777888" {
		t.Fatalf("código após reinício = %q", got)
	}
	if fake.count("GetUpdates") != 2 {
		t.Fatalf("GetUpdates após reinício = %d", fake.count("GetUpdates"))
	}
}

func TestJoinCodeIgnoresOtherGames(t *testing.T) {
	fake := newFakeAMPAPI(t, 20)
	fake.uptime, fake.joinCode = "0:0:5:0", "454389"
	status := collectOne(joinCodeClient(fake), joinCodeInstance(fake, "Minecraft", true))
	if status.JoinCode != "" || fake.count("GetUpdates") != 0 {
		t.Fatalf("jogo não Valheim: código=%q chamadas=%d", status.JoinCode, fake.count("GetUpdates"))
	}
}

func TestJoinCodeClearedWhenStoppedOrIdle(t *testing.T) {
	for _, name := range []string{"parado", "idle"} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeAMPAPI(t, 20)
			fake.uptime, fake.joinCode = "0:0:5:0", "454389"
			client := joinCodeClient(fake)
			instance := joinCodeInstance(fake, "Valheim", true)
			if collectOne(client, instance).JoinCode != "454389" {
				t.Fatal("código deveria ser exibido")
			}

			if name == "parado" {
				instance.Running = false
			} else {
				fake.mutex.Lock()
				fake.state = 0
				fake.mutex.Unlock()
			}
			if got := collectOne(client, instance).JoinCode; got != "" {
				t.Fatalf("código deveria sumir: %q", got)
			}
			client.joinCodeMu.Lock()
			size := len(client.joinCodes)
			client.joinCodeMu.Unlock()
			if size != 0 {
				t.Fatalf("cache deveria ser limpo: %d", size)
			}

			// Novo processo, mesmo uptime alto antes: precisa buscar de novo.
			instance.Running = true
			fake.mutex.Lock()
			fake.state, fake.uptime, fake.joinCode = 20, "0:0:9:0", "222333"
			fake.mutex.Unlock()
			if got := collectOne(client, instance).JoinCode; got != "222333" {
				t.Fatalf("código do novo processo = %q", got)
			}
		})
	}
}

func TestJoinCodeErrorDoesNotBreakCard(t *testing.T) {
	fake := newFakeAMPAPI(t, 20)
	fake.uptime, fake.updatesFail = "0:0:5:0", true
	status := collectOne(joinCodeClient(fake), joinCodeInstance(fake, "Valheim", true))
	if status.JoinCode != "" || status.ApplicationStatus == nil {
		t.Fatalf("status inesperado: %+v", status)
	}
	if text := containerText(buildAMPServerContainer(status, "1.2.3.4")); strings.Contains(text, "convite") {
		t.Fatalf("card não deveria ter código: %s", text)
	}
}

func containerText(container disgoDiscord.ContainerComponent) string {
	var builder strings.Builder
	for _, component := range container.Components {
		if text, ok := component.(disgoDiscord.TextDisplayComponent); ok {
			builder.WriteString(text.Content)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func TestJoinCodeCardRendering(t *testing.T) {
	previous := i18n.Default()
	t.Cleanup(func() { i18n.SetDefault(previous) })

	base := ampInstanceStatusView{Instance: amp.ManagedInstance{Name: "V", Game: "Valheim", Running: true}}
	withCode := base
	withCode.JoinCode = "454389"

	cases := []struct {
		lang    i18n.Language
		address string
		heading string
	}{
		{i18n.PortugueseBrazil, "### Endereço do servidor\n`1.2.3.4`", "### Código de convite\n`454389`"},
		{i18n.EnglishUS, "### Server address\n`1.2.3.4`", "### Join code\n`454389`"},
	}
	for _, test := range cases {
		i18n.SetDefault(test.lang)
		plain := containerText(buildAMPServerContainer(base, "1.2.3.4"))
		text := containerText(buildAMPServerContainer(withCode, "1.2.3.4"))
		if !strings.Contains(text, test.address+"\n\n"+test.heading+"\n\n### ") {
			t.Errorf("%v: seção fora do lugar:\n%s", test.lang, text)
		}
		if strings.Replace(text, "\n\n"+test.heading, "", 1) != plain {
			t.Errorf("%v: sem código o texto deveria ser idêntico", test.lang)
		}
		if strings.Contains(plain, "454389") || strings.Contains(plain, "convite") || strings.Contains(plain, "Join code") {
			t.Errorf("%v: texto sem código vazou seção:\n%s", test.lang, plain)
		}
	}
}

func TestJoinCodeRecheckAfterInterval(t *testing.T) {
	fake := newFakeAMPAPI(t, 20)
	fake.uptime, fake.joinCode = "0:0:5:0", "454389"
	client := joinCodeClient(fake)
	clock := time.Unix(1_700_000_000, 0)
	client.joinCodeNow = func() time.Time { return clock }
	instance := joinCodeInstance(fake, "Valheim", true)
	set := func(uptime, code string) {
		fake.mutex.Lock()
		fake.uptime, fake.joinCode = uptime, code
		fake.mutex.Unlock()
	}

	collectOne(client, instance)

	// Dentro dos 5 minutos: nenhuma leitura extra.
	clock = clock.Add(valheimJoinCodeRecheckInterval - time.Second)
	set("0:0:9:0", "111111")
	if got := collectOne(client, instance).JoinCode; got != "454389" || fake.count("GetUpdates") != 1 {
		t.Fatalf("antes do intervalo: código=%q chamadas=%d", got, fake.count("GetUpdates"))
	}

	// Após 5 minutos: uma leitura e o código novo substitui o antigo.
	clock = clock.Add(2 * time.Second)
	if got := collectOne(client, instance).JoinCode; got != "111111" || fake.count("GetUpdates") != 2 {
		t.Fatalf("após o intervalo: código=%q chamadas=%d", got, fake.count("GetUpdates"))
	}
	collectOne(client, instance)
	if fake.count("GetUpdates") != 2 {
		t.Fatalf("a releitura deveria renovar o relógio: %d", fake.count("GetUpdates"))
	}

	// Após 5 minutos sem linha de código no histórico: mantém o antigo.
	clock = clock.Add(valheimJoinCodeRecheckInterval)
	set("0:0:20:0", "")
	fake.mutex.Lock()
	fake.joinCode = "" // resposta sem dígitos não casa com o padrão
	fake.mutex.Unlock()
	if got := collectOne(client, instance).JoinCode; got != "111111" || fake.count("GetUpdates") != 3 {
		t.Fatalf("sem linha: código=%q chamadas=%d", got, fake.count("GetUpdates"))
	}

	// Erro na releitura: mantém o código em cache.
	clock = clock.Add(valheimJoinCodeRecheckInterval)
	fake.mutex.Lock()
	fake.updatesFail = true
	fake.mutex.Unlock()
	if got := collectOne(client, instance).JoinCode; got != "111111" || fake.count("GetUpdates") != 4 {
		t.Fatalf("com erro: código=%q chamadas=%d", got, fake.count("GetUpdates"))
	}
}
