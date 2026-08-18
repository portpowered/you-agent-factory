package submit

import (
	"context"
	"testing"
)

func TestSubmitCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"batch": func() error {
			return SubmitBatch(testFactoryRequestBatchPreparation{}, BatchConfig{Context: context.Background()})
		},
		"work": func() error {
			return Submit(t, SubmitConfig{Context: context.Background(), Name: "work", WorkTypeName: "task", Payload: "payload.json"})
		},
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}
