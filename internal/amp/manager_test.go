package amp

import (
	"context"
	"testing"
	"time"
)

func TestParseInstancesList(
	t *testing.T,
) {
	t.Parallel()

	output := `
[Info/1] AMP Instance Manager v2.8.0.4

Instance ID        │ ads-id
Module             │ ADS
Instance Name      │ ADS01
Friendly Name      │ ADS01
URL                │ http://127.0.0.1:8080/
Running            │ Yes

Instance ID        │ minecraft-id
Module             │ Minecraft
Instance Name      │ Vanilla-202501
Friendly Name      │ Vanilla - 2025
URL                │ http://127.0.0.1:8081/
Running            │ No

Instance ID        │ palworld-id
Module             │ GenericModule
Instance Name      │ AlamamaPal01
Friendly Name      │ AlamamaPal
URL                │ http://127.0.0.1:8090/
Running            │ Yes
`

	instances, err := parseInstancesList(
		output,
	)
	if err != nil {
		t.Fatalf(
			"parseInstancesList retornou erro: %v",
			err,
		)
	}

	if len(instances) != 3 {
		t.Fatalf(
			"quantidade inesperada de instâncias: obtido %d, esperado 3",
			len(instances),
		)
	}

	if instances[0].Name != "ADS01" {
		t.Errorf(
			"nome inesperado: obtido %q, esperado %q",
			instances[0].Name,
			"ADS01",
		)
	}

	if !instances[0].Running {
		t.Error(
			"ADS01 deveria estar marcada como ativa",
		)
	}

	if instances[1].Name != "Vanilla-202501" {
		t.Errorf(
			"nome inesperado: obtido %q, esperado %q",
			instances[1].Name,
			"Vanilla-202501",
		)
	}

	if instances[1].Running {
		t.Error(
			"Vanilla-202501 deveria estar marcada como parada",
		)
	}

	if instances[2].APIURL != "http://127.0.0.1:8090/" {
		t.Errorf(
			"URL inesperada: obtido %q",
			instances[2].APIURL,
		)
	}
}

func TestParseInstancesListRejectsMissingRunningState(t *testing.T) {
	t.Parallel()

	output := `
Instance ID        â”‚ minecraft-id
Module             â”‚ Minecraft
Instance Name      â”‚ Vanilla-202501
Friendly Name      â”‚ Vanilla - 2025
URL                â”‚ http://127.0.0.1:8081/
`

	if _, err := parseInstancesList(output); err == nil {
		t.Fatal("a saída parcial sem Running deveria ser rejeitada")
	}
}

func TestParseInstancesListRejectsUnknownRunningState(t *testing.T) {
	t.Parallel()

	output := `
Instance ID        â”‚ minecraft-id
Module             â”‚ Minecraft
Instance Name      â”‚ Vanilla-202501
Friendly Name      â”‚ Vanilla - 2025
URL                â”‚ http://127.0.0.1:8081/
Running            â”‚ Unknown
`

	if _, err := parseInstancesList(output); err == nil {
		t.Fatal("um valor Running desconhecido deveria ser rejeitado")
	}
}

func TestNormalizeInstanceURLMarkdown(
	t *testing.T,
) {
	t.Parallel()

	result := normalizeInstanceURL(
		"[http://127.0.0.1:8090/](http://127.0.0.1:8090/)",
	)

	expected := "http://127.0.0.1:8090/"

	if result != expected {
		t.Fatalf(
			"URL inesperada: obtido %q, esperado %q",
			result,
			expected,
		)
	}
}

func TestDiscoverInstancesRealAMP(
	t *testing.T,
) {
	if testing.Short() {
		t.Skip(
			"teste de integração ignorado no modo curto",
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	instances, err := DiscoverInstances(
		ctx,
	)
	if err != nil {
		t.Fatalf(
			"DiscoverInstances retornou erro: %v",
			err,
		)
	}

	if len(instances) == 0 {
		t.Fatal(
			"nenhuma instância controlável foi encontrada",
		)
	}

	for _, instance := range instances {
		if instance.Name == "ADS01" {
			t.Fatal(
				"ADS01 não deveria aparecer entre as instâncias controláveis",
			)
		}

		if instance.Name == "" {
			t.Fatal(
				"foi encontrada uma instância sem nome",
			)
		}

		if instance.FriendlyName == "" {
			t.Fatalf(
				"a instância %q está sem nome amigável",
				instance.Name,
			)
		}

		t.Logf(
			"instância=%s amigável=%s módulo=%s ativa=%t url=%s",
			instance.Name,
			instance.FriendlyName,
			instance.Module,
			instance.Running,
			instance.APIURL,
		)
	}
}
