package amp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestApplicationStatusPlayerCounts(t *testing.T) {
	status := ApplicationStatus{
		Metrics: map[string]StatusMetric{
			"Active Users": {
				RawValue: 3,
				MaxValue: 32,
			},
		},
	}

	counts, err := status.PlayerCounts()
	if err != nil {
		t.Fatalf("PlayerCounts retornou erro: %v", err)
	}

	if counts.Current != 3 || counts.Maximum != 32 {
		t.Fatalf("contagem inesperada: %+v", counts)
	}
}

func TestDiscoverManagedInstancesUsesADSMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/API/Core/Login", func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"success":   true,
			"sessionID": "test-session",
		})
	})
	mux.HandleFunc("/API/ADSModule/GetInstances", func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("sessão não foi enviada no inventário ADS")
		}

		_ = json.NewEncoder(writer).Encode([]map[string]any{
			{
				"AvailableInstances": []map[string]any{
					{
						"InstanceID":   "ads-id",
						"InstanceName": "ADS01",
						"Module":       "ADS",
						"IP":           "127.0.0.1",
						"Port":         8080,
						"Running":      true,
					},
					{
						"InstanceID":        "pal-id",
						"InstanceName":      "AlamamaPal01",
						"FriendlyName":      "Alamama",
						"Module":            "GenericModule",
						"ModuleDisplayName": "Palworld",
						"IP":                "127.0.0.1",
						"Port":              8090,
						"Running":           true,
					},
				},
			},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := NewAPIClient("user", "password")
	instances, err := client.DiscoverManagedInstances(
		context.Background(),
		server.URL,
	)
	if err != nil {
		t.Fatalf("DiscoverManagedInstances retornou erro: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("quantidade inesperada de instâncias: %d", len(instances))
	}

	instance := instances[0]
	if instance.Name != "AlamamaPal01" ||
		instance.Game != "Palworld" ||
		instance.APIURL != "http://127.0.0.1:8090/" ||
		!instance.Running {
		t.Fatalf("instância inesperada: %+v", instance)
	}
}

func TestApplicationStatusPlayerCountsRejectsMissingMetric(t *testing.T) {
	_, err := (ApplicationStatus{}).PlayerCounts()
	if err == nil {
		t.Fatal("era esperado erro para métrica ausente")
	}

	if !strings.Contains(err.Error(), "não foi retornada") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestApplicationStatusPlayerCountsRejectsInvalidValues(t *testing.T) {
	tests := []StatusMetric{
		{RawValue: -1, MaxValue: 32},
		{RawValue: 0, MaxValue: 0},
		{RawValue: 33, MaxValue: 32},
		{RawValue: 1.5, MaxValue: 32},
	}

	for _, metric := range tests {
		status := ApplicationStatus{
			Metrics: map[string]StatusMetric{
				"Active Users": metric,
			},
		}

		if _, err := status.PlayerCounts(); err == nil {
			t.Fatalf("era esperado erro para métrica inválida: %+v", metric)
		}
	}
}

func TestManagedInstanceGame(t *testing.T) {
	tests := []struct {
		name     string
		instance adsManagedInstance
		expected string
	}{
		{
			name: "generic module display name",
			instance: adsManagedInstance{
				Module:            "GenericModule",
				ModuleDisplayName: "Palworld",
			},
			expected: "Palworld",
		},
		{
			name: "minecraft module",
			instance: adsManagedInstance{
				Module: "Minecraft",
			},
			expected: "Minecraft",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if actual := managedInstanceGame(test.instance); actual != test.expected {
				t.Fatalf("jogo inesperado: %q", actual)
			}
		})
	}
}

func TestManagedInstanceAPIURL(t *testing.T) {
	actual := managedInstanceAPIURL(adsManagedInstance{
		IP:   "127.0.0.1",
		Port: 8090,
	})

	if actual != "http://127.0.0.1:8090/" {
		t.Fatalf("URL inesperada: %q", actual)
	}
}
