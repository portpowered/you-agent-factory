package modelhost

import (
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestLocalInvocationEndpointPreservesCommandAndSupervisedSelection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		backend     string
		args        []string
		diagnostics map[string]string
		want        string
		notReady    bool
	}{
		{name: "unmanaged command", backend: "unmanaged", diagnostics: map[string]string{"endpoint": "http://ignored"}},
		{name: "managed command", backend: "LLAMACPP"},
		{name: "missing flag value", backend: "LLAMACPP", args: []string{"--health-endpoint"}},
		{name: "empty flag value", backend: "LLAMACPP", args: []string{"--health-endpoint", " "}},
		{name: "supervised missing endpoint", backend: "LLAMACPP", args: []string{"--health-endpoint", "http://health"}, notReady: true},
		{name: "supervised empty endpoint", backend: "LLAMACPP", args: []string{"--health-endpoint", "http://health"}, diagnostics: map[string]string{"endpoint": " "}, notReady: true},
		{name: "supervised selected endpoint", backend: "LLAMACPP", args: []string{"--health-endpoint", "http://health"}, diagnostics: map[string]string{"endpoint": " http://selected "}, want: "http://selected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			config := &models.RuntimeConfig{
				Resources: []models.RuntimeResource{{Type: models.RuntimeResourceTypeModel, Model: " Model ", Backend: test.backend}},
				Workers:   []models.RuntimeWorker{{Model: "model", ModelLocality: models.RuntimeModelLocalityLocal, Args: test.args}},
			}
			got, err := LocalInvocationEndpoint(config, "MODEL", test.diagnostics)
			if got != test.want || errors.Is(err, models.ErrHostRuntimeNotReady) != test.notReady || (err != nil && !test.notReady) {
				t.Fatalf("endpoint = %q, error = %v; want %q, notReady=%t", got, err, test.want, test.notReady)
			}
		})
	}
}

func TestLocalInvocationEndpointRequiresLocalWorkerOnlyForManagedResource(t *testing.T) {
	t.Parallel()
	for _, config := range []*models.RuntimeConfig{nil, {}} {
		if endpoint, err := LocalInvocationEndpoint(config, "model", nil); endpoint != "" || err != nil {
			t.Fatalf("no managed resource: endpoint=%q, error=%v", endpoint, err)
		}
	}
	config := &models.RuntimeConfig{
		Resources: []models.RuntimeResource{{Type: models.RuntimeResourceTypeModel, Model: "model", Backend: "LLAMACPP"}},
		Workers:   []models.RuntimeWorker{{Model: "model", ModelLocality: models.RuntimeModelLocalityCloud}},
	}
	if endpoint, err := LocalInvocationEndpoint(config, "model", nil); endpoint != "" || err == nil {
		t.Fatalf("missing local worker: endpoint=%q, error=%v", endpoint, err)
	}
}
