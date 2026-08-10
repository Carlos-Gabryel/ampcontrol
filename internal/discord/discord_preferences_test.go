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
		InstanceSettings: []instancePresentationOverride{
			{Instance: "Valheim01", DisplayName: "Valheim Brasil", Game: "Valheim", MaximumPlayers: 20},
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
	if len(actual.InstanceSettings) != 1 ||
		actual.InstanceSettings[0].DisplayName != "Valheim Brasil" ||
		actual.InstanceSettings[0].MaximumPlayers != 20 {
		t.Fatalf("configuração de apresentação inesperada: %#v", actual.InstanceSettings)
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
			InstanceSettings: []instancePresentationOverride{
				{Instance: "Valheim01", Game: "Valheim"},
			},
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
	if len(stored.InstanceSettings) != 1 || stored.InstanceSettings[0].Game != "Valheim" {
		t.Fatal("alterar visibilidade não deveria apagar as configurações da instância")
	}

	changed, err = client.setInstanceHidden("Valheim01", false)
	if err != nil || !changed || client.instanceHidden("Valheim01") {
		t.Fatalf("não foi possível voltar a exibir: changed=%v err=%v", changed, err)
	}
}

func TestClientInstancePresentationSettingsPersistAndApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "discord_preferences.json")
	client := &Client{
		preferencesPath: path,
		preferences: discordPreferences{
			HiddenInstances:  []string{},
			InstanceSettings: []instancePresentationOverride{},
		},
	}
	name := "Walking Rats Brasil"
	game := "Project Zomboid"
	maximum := 32
	setting, err := client.setInstancePresentationSettings(
		"TheWalkingRats01", &name, &game, &maximum,
	)
	if err != nil || setting.MaximumPlayers != 32 {
		t.Fatalf("não foi possível salvar a apresentação: %#v err=%v", setting, err)
	}

	instances := []amp.ManagedInstance{{
		Name: "TheWalkingRats01", FriendlyName: "Original", Game: "Generic Module",
	}}
	applyInstancePresentationOverrides(instances, client.instancePresentationSettingsSnapshot())
	if instances[0].FriendlyName != name || instances[0].Game != game {
		t.Fatalf("override não foi aplicado: %#v", instances[0])
	}

	removed, err := client.resetInstancePresentationSettings("thewalkingrats01")
	if err != nil || !removed {
		t.Fatalf("não foi possível restaurar a apresentação: removed=%v err=%v", removed, err)
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
	foundConfigure := false
	foundDetails := false
	foundRestore := false
	for _, option := range command.Options {
		if option.OptionName() == "idle-adicionar" {
			foundIdleAdd = true
		}
		if option.OptionName() == "diagnostico" {
			foundDiagnostics = true
		}
		if option.OptionName() == "configurar" {
			foundConfigure = true
		}
		if option.OptionName() == "detalhes" {
			foundDetails = true
		}
		if option.OptionName() == "restaurar" {
			foundRestore = true
		}
	}
	if !foundIdleAdd {
		t.Fatal("ampconfig deveria oferecer o cadastro no Idle")
	}
	if !foundDiagnostics {
		t.Fatal("ampconfig deveria oferecer o diagnóstico privado")
	}
	if !foundConfigure || !foundDetails || !foundRestore {
		t.Fatal("ampconfig deveria oferecer gerenciamento completo das instâncias")
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
