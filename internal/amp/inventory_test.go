package amp

import (
	"context"
	"errors"
	"testing"
)

type fakeADSInventory struct {
	instances []ManagedInstance
	err       error
}

func (f fakeADSInventory) DiscoverManagedInstances(context.Context, string) ([]ManagedInstance, error) {
	return f.instances, f.err
}

func TestInventoryPrefersADS(t *testing.T) {
	inventory, err := NewInventory(fakeADSInventory{instances: []ManagedInstance{{Name: "Game01"}}}, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	inventory.fallback = func(context.Context) ([]ManagedInstance, error) {
		t.Fatal("o fallback não deveria ser chamado")
		return nil, nil
	}
	instances, err := inventory.DiscoverInstances(context.Background())
	if err != nil || len(instances) != 1 || instances[0].Name != "Game01" {
		t.Fatalf("inventário inesperado: %#v erro=%v", instances, err)
	}
}

func TestInventoryFallsBackToManager(t *testing.T) {
	inventory, err := NewInventory(fakeADSInventory{err: errors.New("ADS indisponível")}, "http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	inventory.fallback = func(context.Context) ([]ManagedInstance, error) {
		return []ManagedInstance{{Name: "Minecraft01", Module: "Minecraft"}}, nil
	}
	instances, err := inventory.DiscoverInstances(context.Background())
	if err != nil || len(instances) != 1 || instances[0].Game != "Minecraft" {
		t.Fatalf("fallback inesperado: %#v erro=%v", instances, err)
	}
}
