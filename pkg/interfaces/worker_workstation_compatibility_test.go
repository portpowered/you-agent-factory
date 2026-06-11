package interfaces

import "testing"

func TestPublicWorkerTypeForFactoryUsage(t *testing.T) {
	t.Parallel()

	agentWorker := WorkerConfig{Name: "executor", Type: WorkerTypeModel}
	agentWorkstations := []FactoryWorkstationConfig{{
		Name:           "execute-story",
		Type:           WorkstationTypeModel,
		WorkerTypeName: "executor",
	}}

	if got := PublicWorkerTypeForFactoryUsage(agentWorker, agentWorkstations); got != WorkerTypeAgent {
		t.Fatalf("agent factory usage = %q, want %q", got, WorkerTypeAgent)
	}

	inferenceWorker := WorkerConfig{Name: "executor", Type: WorkerTypeModel}
	inferenceWorkstations := []FactoryWorkstationConfig{{
		Name:           "invoke-story",
		Type:           WorkstationTypeInvoke,
		WorkerTypeName: "executor",
	}}
	if got := PublicWorkerTypeForFactoryUsage(inferenceWorker, inferenceWorkstations); got != WorkerTypeInference {
		t.Fatalf("inference factory usage = %q, want %q", got, WorkerTypeInference)
	}
}

func TestWorkerMatchesWorkstationBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		workerType  string
		workstation FactoryWorkstationConfig
		want        bool
	}{
		{
			name:       "inference run with inference worker",
			workerType: WorkerTypeInference,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeInference,
			},
			want: true,
		},
		{
			name:       "legacy invoke with model worker",
			workerType: WorkerTypeModel,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeInvoke,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with model worker",
			workerType: WorkerTypeModel,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "agent run with agent worker",
			workerType: WorkerTypeAgent,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeAgent,
			},
			want: true,
		},
		{
			name:       "script run with script worker",
			workerType: WorkerTypeScript,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeScript,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with script worker",
			workerType: WorkerTypeScript,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "poller run with poller worker",
			workerType: WorkerTypePoller,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypePoller,
				Kind: WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "legacy hosted worker on poller workstation",
			workerType: WorkerTypeHosted,
			workstation: FactoryWorkstationConfig{
				Kind: WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "agent run with inference worker",
			workerType: WorkerTypeInference,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "agent run with legacy model worker",
			workerType: WorkerTypeModel,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "inference run with agent worker",
			workerType: WorkerTypeAgent,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeInference,
			},
			want: false,
		},
		{
			name:       "poller run with inference worker",
			workerType: WorkerTypeInference,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypePoller,
				Kind: WorkstationKindPoller,
			},
			want: false,
		},
		{
			name:       "logical move exempt",
			workerType: WorkerTypeInference,
			workstation: FactoryWorkstationConfig{
				Type: WorkstationTypeLogical,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := WorkerMatchesWorkstationBehavior(tt.workerType, tt.workstation); got != tt.want {
				t.Fatalf("WorkerMatchesWorkstationBehavior(%q, %#v) = %v, want %v", tt.workerType, tt.workstation, got, tt.want)
			}
		})
	}
}
