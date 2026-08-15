package workers_test

import (
	"testing"

	workers "github.com/portpowered/infinite-you/pkg/services/workers"
)

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
