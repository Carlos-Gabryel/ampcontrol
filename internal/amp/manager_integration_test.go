//go:build integration

package amp

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDiscoverInstancesRealAMP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	instances, err := DiscoverInstances(ctx)
	if err != nil {
		t.Fatalf("DiscoverInstances retornou erro: %v", err)
	}
	if len(instances) == 0 {
		t.Fatal("nenhuma instância controlável foi encontrada")
	}
	for _, instance := range instances {
		if instance.Name == "ADS01" || instance.Name == "" || instance.FriendlyName == "" {
			t.Fatalf("instância AMP inválida descoberta: %#v", instance)
		}
	}
}

func TestControlInstanceRejectsUnknownInstance(t *testing.T) {
	err := RestartInstance(context.Background(), "ServidorQueNaoExiste")
	if err == nil || !strings.Contains(err.Error(), "não existe") {
		t.Fatalf("erro inesperado para instância inexistente: %v", err)
	}
}
