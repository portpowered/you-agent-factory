package executor

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestResolveExecutionTimeoutDefaultsForMissingOrTypelessWorker(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		worker *interfaces.FactoryWorkerConfig
	}{
		{name: "missing worker", worker: nil},
		{name: "typeless worker", worker: &interfaces.FactoryWorkerConfig{}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveExecutionTimeout(nil, test.worker, nil)
			if err != nil {
				t.Fatalf("resolveExecutionTimeout() error = %v", err)
			}
			if got != defaultSubprocessExecutionTimeout {
				t.Fatalf("resolveExecutionTimeout() = %v, want %v", got, defaultSubprocessExecutionTimeout)
			}
		})
	}
}
