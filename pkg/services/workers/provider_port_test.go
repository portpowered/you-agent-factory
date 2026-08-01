package workers_test

import (
	"context"
	"testing"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

type rootRunnerStub struct{}

func (rootRunnerStub) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.InferenceResponse, error) {
	return workers.InferenceResponse{}, nil
}

func TestRunnerPortExposedAtWorkersRoot(t *testing.T) {
	t.Parallel()

	var runner workers.Runner = rootRunnerStub{}
	if runner == nil {
		t.Fatal("workers.Runner assignment failed")
	}
}

func TestRunnerIdentityForWorker(t *testing.T) {
	tests := []struct {
		name, executor, modelProvider, want string
		wantErr                             bool
	}{
		{name: "canonical ACP", executor: "ACP", modelProvider: "cursor-acp", want: "cursor-acp"},
		{name: "canonical ACP is case insensitive", executor: "acp", modelProvider: "custom", want: "custom"},
		{name: "legacy named executor", executor: "cursor-acp", want: "cursor-acp"},
		{name: "script wrap", executor: "SCRIPT_WRAP"},
		{name: "missing ACP integration", executor: "ACP", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := workers.RunnerIdentityForWorker(test.executor, test.modelProvider)
			if (err != nil) != test.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("identity = %q, want %q", got, test.want)
			}
		})
	}
}
