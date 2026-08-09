package discord

import (
	"testing"

	"github.com/alabamaamp/ampcontrol/internal/amp"
	"github.com/disgoorg/snowflake/v2"
)

func TestBuildAMPChoiceNameShowsGameInsteadOfInstance(t *testing.T) {
	instance := amp.ManagedInstance{
		Name:         "TheWalkingRats01",
		FriendlyName: "The Walking Rats",
		Module:       "GenericModule",
		Game:         "Project Zomboid",
	}

	if actual := buildAMPChoiceName(instance); actual !=
		"The Walking Rats — Project Zomboid" {
		t.Fatalf("nome da opção inesperado: %q", actual)
	}
}

func TestApplyCommandGameNamesUsesConfiguredOverride(t *testing.T) {
	instances := []amp.ManagedInstance{
		{
			Name:   "TheWalkingRats01",
			Module: "GenericModule",
		},
	}

	applyCommandGameNames(
		instances,
		map[string]string{
			"thewalkingrats01": "Project Zomboid",
		},
	)

	if instances[0].Game != "Project Zomboid" {
		t.Fatalf("jogo inesperado: %q", instances[0].Game)
	}
}

func TestAMPCommandChannelAllowedRequiresConfiguredChannel(t *testing.T) {
	allowed := snowflake.ID(123)
	if !ampCommandChannelAllowed(allowed, allowed) {
		t.Fatal("o canal configurado deveria aceitar comandos")
	}
	if ampCommandChannelAllowed(snowflake.ID(456), allowed) {
		t.Fatal("outro canal não deveria aceitar comandos")
	}
	if ampCommandChannelAllowed(0, allowed) {
		t.Fatal("uma interação sem canal não deveria ser aceita")
	}
}
