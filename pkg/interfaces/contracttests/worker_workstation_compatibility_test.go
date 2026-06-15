package contracttests

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestPublicWorkerTypeForFactoryUsage(t *testing.T) {
	t.Parallel()

	agentWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	agentWorkstations := []interfaces.FactoryWorkstationConfig{{
		Name:           "execute-story",
		Type:           interfaces.WorkstationTypeModel,
		WorkerTypeName: "executor",
	}}

	if got := interfaces.PublicWorkerTypeForFactoryUsage(agentWorker, agentWorkstations); got != interfaces.WorkerTypeAgent {
		t.Fatalf("agent factory usage = %q, want %q", got, interfaces.WorkerTypeAgent)
	}

	inferenceWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	inferenceWorkstations := []interfaces.FactoryWorkstationConfig{{
		Name:           "invoke-story",
		Type:           interfaces.WorkstationTypeInvoke,
		WorkerTypeName: "executor",
	}}
	if got := interfaces.PublicWorkerTypeForFactoryUsage(inferenceWorker, inferenceWorkstations); got != interfaces.WorkerTypeInference {
		t.Fatalf("inference factory usage = %q, want %q", got, interfaces.WorkerTypeInference)
	}

	mixedWorker := interfaces.WorkerConfig{Name: "executor", Type: interfaces.WorkerTypeModel}
	mixedWorkstations := []interfaces.FactoryWorkstationConfig{
		{
			Name:           "execute-story",
			Type:           interfaces.WorkstationTypeModel,
			WorkerTypeName: "executor",
		},
		{
			Name:           "invoke-story",
			Type:           interfaces.WorkstationTypeInvoke,
			WorkerTypeName: "executor",
		},
	}
	if got := interfaces.PublicWorkerTypeForFactoryUsage(mixedWorker, mixedWorkstations); got != interfaces.WorkerTypeModel {
		t.Fatalf("mixed legacy factory usage = %q, want %q", got, interfaces.WorkerTypeModel)
	}
}

type workerWorkstationBehaviorCase struct {
	name        string
	workerType  string
	workstation interfaces.FactoryWorkstationConfig
	want        bool
}

func workerMatchesWorkstationBehaviorCompatibleCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:       "inference run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInference,
			},
			want: true,
		},
		{
			name:       "legacy invoke with model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInvoke,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "agent run with agent worker",
			workerType: interfaces.WorkerTypeAgent,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: true,
		},
		{
			name:       "script run with script worker",
			workerType: interfaces.WorkerTypeScript,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeScript,
			},
			want: true,
		},
		{
			name:       "legacy model workstation with script worker",
			workerType: interfaces.WorkerTypeScript,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeModel,
			},
			want: true,
		},
		{
			name:       "poller run with poller worker",
			workerType: interfaces.WorkerTypePoller,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypePoller,
				Kind: interfaces.WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "legacy hosted worker on poller workstation",
			workerType: interfaces.WorkerTypeHosted,
			workstation: interfaces.FactoryWorkstationConfig{
				Kind: interfaces.WorkstationKindPoller,
			},
			want: true,
		},
		{
			name:       "logical move exempt",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeLogical,
			},
			want: true,
		},
	}
}

func workerMatchesWorkstationBehaviorIncompatibleCases() []workerWorkstationBehaviorCase {
	return []workerWorkstationBehaviorCase{
		{
			name:       "agent run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "agent run with legacy model worker",
			workerType: interfaces.WorkerTypeModel,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeAgent,
			},
			want: false,
		},
		{
			name:       "inference run with agent worker",
			workerType: interfaces.WorkerTypeAgent,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypeInference,
			},
			want: false,
		},
		{
			name:       "poller run with inference worker",
			workerType: interfaces.WorkerTypeInference,
			workstation: interfaces.FactoryWorkstationConfig{
				Type: interfaces.WorkstationTypePoller,
				Kind: interfaces.WorkstationKindPoller,
			},
			want: false,
		},
	}
}

func workerMatchesWorkstationBehaviorCases() []workerWorkstationBehaviorCase {
	cases := workerMatchesWorkstationBehaviorCompatibleCases()
	cases = append(cases, workerMatchesWorkstationBehaviorIncompatibleCases()...)
	return cases
}

func TestWorkerMatchesWorkstationBehavior(t *testing.T) {
	t.Parallel()

	for _, tt := range workerMatchesWorkstationBehaviorCases() {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := interfaces.WorkerMatchesWorkstationBehavior(tt.workerType, tt.workstation); got != tt.want {
				t.Fatalf("WorkerMatchesWorkstationBehavior(%q, %#v) = %v, want %v", tt.workerType, tt.workstation, got, tt.want)
			}
		})
	}
}
