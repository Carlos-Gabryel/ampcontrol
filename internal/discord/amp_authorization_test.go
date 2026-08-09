package discord

import (
	"testing"

	disgoDiscord "github.com/disgoorg/disgo/discord"
)

func TestAMPCommandAuthorizedRequiresAdministrator(t *testing.T) {
	if ampCommandAuthorized(nil) {
		t.Fatal("interação sem membro não deve ser autorizada")
	}

	regularMember := &disgoDiscord.ResolvedMember{
		Permissions: disgoDiscord.PermissionManageMessages,
	}
	if ampCommandAuthorized(regularMember) {
		t.Fatal("membro sem Administrator não deve ser autorizado")
	}

	administrator := &disgoDiscord.ResolvedMember{
		Permissions: disgoDiscord.PermissionAdministrator,
	}
	if !ampCommandAuthorized(administrator) {
		t.Fatal("administrador deveria ser autorizado")
	}
}
