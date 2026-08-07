package discord

import (
	"testing"

	"github.com/alabamaamp/palcontrol/internal/amp"
)

func TestPartitionLegacyIdleServersKeepsAllWithoutTransfers(
	t *testing.T,
) {
	t.Parallel()

	servers := []managedServer{
		{
			DisplayName: "Alamama",
			Instance: amp.Instance{
				Name: "AlamamaPal01",
			},
		},
		{
			DisplayName: "Kalaga",
			Instance: amp.Instance{
				Name: "KalagaPal01",
			},
		},
	}

	managed, transferred := partitionLegacyIdleServers(
		servers,
		nil,
	)

	if len(managed) != 2 {
		t.Fatalf(
			"eram esperados 2 servidores sob autoridade legada; encontrados=%d",
			len(managed),
		)
	}

	if len(transferred) != 0 {
		t.Fatalf(
			"nenhum servidor deveria ter sido transferido; encontrados=%d",
			len(transferred),
		)
	}
}

func TestPartitionLegacyIdleServersTransfersOnlySelectedInstance(
	t *testing.T,
) {
	t.Parallel()

	servers := []managedServer{
		{
			DisplayName: "Alamama",
			Instance: amp.Instance{
				Name: "AlamamaPal01",
			},
		},
		{
			DisplayName: "Kalaga",
			Instance: amp.Instance{
				Name: "KalagaPal01",
			},
		},
	}

	managed, transferred := partitionLegacyIdleServers(
		servers,
		[]string{
			"AlamamaPal01",
		},
	)

	if len(managed) != 1 {
		t.Fatalf(
			"era esperado 1 servidor sob autoridade legada; encontrados=%d",
			len(managed),
		)
	}

	if managed[0].Instance.Name != "KalagaPal01" {
		t.Fatalf(
			"servidor legado inesperado: %s",
			managed[0].Instance.Name,
		)
	}

	if len(transferred) != 1 {
		t.Fatalf(
			"era esperado 1 servidor transferido; encontrados=%d",
			len(transferred),
		)
	}

	if transferred[0].Instance.Name != "AlamamaPal01" {
		t.Fatalf(
			"servidor transferido inesperado: %s",
			transferred[0].Instance.Name,
		)
	}
}

func TestPartitionLegacyIdleServersTransfersBothInstances(
	t *testing.T,
) {
	t.Parallel()

	servers := []managedServer{
		{
			DisplayName: "Alamama",
			Instance: amp.Instance{
				Name: "AlamamaPal01",
			},
		},
		{
			DisplayName: "Kalaga",
			Instance: amp.Instance{
				Name: "KalagaPal01",
			},
		},
	}

	managed, transferred := partitionLegacyIdleServers(
		servers,
		[]string{
			"AlamamaPal01",
			"KalagaPal01",
		},
	)

	if len(managed) != 0 {
		t.Fatalf(
			"nenhum servidor deveria permanecer sob autoridade legada; encontrados=%d",
			len(managed),
		)
	}

	if len(transferred) != 2 {
		t.Fatalf(
			"eram esperados 2 servidores transferidos; encontrados=%d",
			len(transferred),
		)
	}
}

func TestPartitionLegacyIdleServersNormalizesTransferredNames(
	t *testing.T,
) {
	t.Parallel()

	servers := []managedServer{
		{
			DisplayName: "Alamama",
			Instance: amp.Instance{
				Name: "AlamamaPal01",
			},
		},
		{
			DisplayName: "Kalaga",
			Instance: amp.Instance{
				Name: "KalagaPal01",
			},
		},
	}

	managed, transferred := partitionLegacyIdleServers(
		servers,
		[]string{
			"  AlamamaPal01  ",
			"",
			"ServidorQueNaoExiste",
			"AlamamaPal01",
		},
	)

	if len(managed) != 1 {
		t.Fatalf(
			"era esperado 1 servidor legado; encontrados=%d",
			len(managed),
		)
	}

	if managed[0].Instance.Name != "KalagaPal01" {
		t.Fatalf(
			"servidor legado inesperado: %s",
			managed[0].Instance.Name,
		)
	}

	if len(transferred) != 1 {
		t.Fatalf(
			"era esperado somente Alamama transferido; encontrados=%d",
			len(transferred),
		)
	}

	if transferred[0].Instance.Name != "AlamamaPal01" {
		t.Fatalf(
			"servidor transferido inesperado: %s",
			transferred[0].Instance.Name,
		)
	}
}
