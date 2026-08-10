package discord

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	disgoDiscord "github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestDiscordPreferencesRoundTripNormalizesHiddenInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord_preferences.json")
	expected := []string{"AlamamaPal01", "Vanilla-202501"}

	err := saveDiscordPreferences(path, discordPreferences{
		HiddenInstances: []string{
			" Vanilla-202501 ",
			"AlamamaPal01",
			"alamamapal01",
			"",
		},
	})
	if err != nil {
		t.Fatalf("saveDiscordPreferences retornou erro: %v", err)
	}

	actual, err := loadDiscordPreferences(path)
	if err != nil {
		t.Fatalf("loadDiscordPreferences retornou erro: %v", err)
	}
	if !reflect.DeepEqual(actual.HiddenInstances, expected) {
		t.Fatalf(
			"instâncias ocultas inesperadas: %#v",
			actual.HiddenInstances,
		)
	}
}

func TestSplitAMPInstancesByVisibilityIncludesStalePreferences(t *testing.T) {
	instances := []amp.ManagedInstance{
		{Name: "AIO01", FriendlyName: "AIO"},
		{Name: "AlamamaPal01", FriendlyName: "Alamama"},
		{Name: "Valheim01", FriendlyName: "Valheim"},
	}

	visible, hidden := splitAMPInstancesByVisibility(
		instances,
		[]string{"alamamapal01", "Removida01"},
	)

	if len(visible) != 2 ||
		visible[0].Name != "AIO01" ||
		visible[1].Name != "Valheim01" {
		t.Fatalf("instâncias visíveis inesperadas: %#v", visible)
	}
	if len(hidden) != 2 ||
		hidden[0].Name != "AlamamaPal01" ||
		hidden[1].Name != "Removida01" {
		t.Fatalf("instâncias ocultas inesperadas: %#v", hidden)
	}
}

func TestClientSetInstanceHiddenPersistsImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord_preferences.json")
	client := &Client{
		preferencesPath: path,
		preferences: discordPreferences{
			HiddenInstances: []string{},
		},
	}

	changed, err := client.setInstanceHidden("Valheim01", true)
	if err != nil || !changed {
		t.Fatalf("não foi possível ocultar: changed=%v err=%v", changed, err)
	}
	if !client.instanceHidden("valheim01") {
		t.Fatal("a instância deveria estar oculta sem diferenciar maiúsculas")
	}

	stored, err := loadDiscordPreferences(path)
	if err != nil {
		t.Fatalf("não foi possível reler a preferência: %v", err)
	}
	if !reflect.DeepEqual(stored.HiddenInstances, []string{"Valheim01"}) {
		t.Fatalf("preferência persistida inesperada: %#v", stored)
	}

	changed, err = client.setInstanceHidden("Valheim01", false)
	if err != nil || !changed || client.instanceHidden("Valheim01") {
		t.Fatalf("não foi possível voltar a exibir: changed=%v err=%v", changed, err)
	}
}

func TestAMPConfigAuthorizationRequiresExactOwnerAndAdministrator(t *testing.T) {
	ownerID := snowflake.ID(228297467865595904)
	administrator := &disgoDiscord.ResolvedMember{
		Permissions: disgoDiscord.PermissionAdministrator,
	}

	if !ampConfigCommandAuthorized(ownerID, administrator, ownerID) {
		t.Fatal("o proprietário administrador deveria ser autorizado")
	}
	if ampConfigCommandAuthorized(snowflake.ID(123), administrator, ownerID) {
		t.Fatal("outro administrador não deveria ser autorizado")
	}
	if ampConfigCommandAuthorized(ownerID, &disgoDiscord.ResolvedMember{}, ownerID) {
		t.Fatal("o ID correto sem permissão administrativa não deveria ser autorizado")
	}
}

func TestAMPConfigCommandDefaultsToAdministrators(t *testing.T) {
	command := buildAMPConfigCommand(nil, nil, nil)
	permissions := command.DefaultMemberPermissions
	if !permissions.OK || permissions.Value == nil ||
		!permissions.Value.Has(disgoDiscord.PermissionAdministrator) {
		t.Fatal("ampconfig deveria ficar oculto por padrão para não administradores")
	}

	foundIdleAdd := false
	foundDiagnostics := false
	for _, option := range command.Options {
		if option.OptionName() == "idle-adicionar" {
			foundIdleAdd = true
		}
		if option.OptionName() == "diagnostico" {
			foundDiagnostics = true
		}
	}
	if !foundIdleAdd {
		t.Fatal("ampconfig deveria oferecer o cadastro no Idle")
	}
	if !foundDiagnostics {
		t.Fatal("ampconfig deveria oferecer o diagnóstico privado")
	}
}

func TestFilterUnregisteredAMPInstances(t *testing.T) {
	instances := []amp.ManagedInstance{
		{Name: "AIO01"},
		{Name: "NovoServidor01"},
		{Name: "Valheim01"},
	}

	actual := filterUnregisteredAMPInstances(
		instances,
		[]string{"aio01", "Valheim01"},
	)
	if len(actual) != 1 || actual[0].Name != "NovoServidor01" {
		t.Fatalf("candidatos ao Idle inesperados: %#v", actual)
	}
}

func TestAMPInstanceInventorySignatureIgnoresOrderAndCase(t *testing.T) {
	left := ampInstanceInventorySignature([]amp.ManagedInstance{
		{Name: "Valheim01"},
		{Name: "AIO01"},
	})
	right := ampInstanceInventorySignature([]amp.ManagedInstance{
		{Name: "aio01"},
		{Name: "VALHEIM01"},
	})
	if left != right {
		t.Fatalf("assinaturas equivalentes divergiram: %q != %q", left, right)
	}
}
