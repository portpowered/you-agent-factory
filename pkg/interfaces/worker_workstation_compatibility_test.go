package interfaces

import (
	"strings"
	"testing"
)

func TestCompatibleWorkerWorkstationBehavior(t *testing.T) {
	for _, tt := range compatibleWorkerWorkstationBehaviorCases() {
		t.Run(tt.name, func(t *testing.T) {
			got := CompatibleWorkerWorkstationBehavior(tt.workerType, tt.workstationType, tt.kind)
			if got != tt.wantCompatible {
				t.Fatalf("CompatibleWorkerWorkstationBehavior(%q, %q, %q) = %v, want %v",
					tt.workerType, tt.workstationType, tt.kind, got, tt.wantCompatible)
			}
		})
	}
}

func compatibleWorkerWorkstationBehaviorCases() []struct {
	name            string
	workerType      string
	workstationType string
	kind            WorkstationKind
	wantCompatible  bool
} {
	return []struct {
		name            string
		workerType      string
		workstationType string
		kind            WorkstationKind
		wantCompatible  bool
	}{
		{
			name:            "inference run with inference worker",
			workerType:      WorkerTypeInference,
			workstationType: WorkstationTypeInference,
			wantCompatible:  true,
		},
		{
			name:            "legacy model invoke with model worker",
			workerType:      WorkerTypeModel,
			workstationType: WorkstationTypeInvoke,
			wantCompatible:  true,
		},
		{
			name:            "agent run with agent worker",
			workerType:      WorkerTypeAgent,
			workstationType: WorkstationTypeAgent,
			wantCompatible:  true,
		},
		{
			name:            "legacy model workstation with model worker",
			workerType:      WorkerTypeModel,
			workstationType: WorkstationTypeModel,
			wantCompatible:  true,
		},
		{
			name:            "legacy model workstation with script worker",
			workerType:      WorkerTypeScript,
			workstationType: WorkstationTypeModel,
			wantCompatible:  true,
		},
		{
			name:            "script run with script worker",
			workerType:      WorkerTypeScript,
			workstationType: WorkstationTypeScript,
			wantCompatible:  true,
		},
		{
			name:            "poller run with poller worker",
			workerType:      WorkerTypePoller,
			workstationType: WorkstationTypePoller,
			wantCompatible:  true,
		},
		{
			name:            "legacy poller kind with hosted worker",
			workerType:      WorkerTypeHosted,
			workstationType: "",
			kind:            WorkstationKindPoller,
			wantCompatible:  true,
		},
		{
			name:            "legacy poller kind with script worker",
			workerType:      WorkerTypeScript,
			workstationType: "",
			kind:            WorkstationKindPoller,
			wantCompatible:  true,
		},
		{
			name:            "agent run with inference worker",
			workerType:      WorkerTypeInference,
			workstationType: WorkstationTypeAgent,
			wantCompatible:  false,
		},
		{
			name:            "inference run with agent worker",
			workerType:      WorkerTypeAgent,
			workstationType: WorkstationTypeInference,
			wantCompatible:  false,
		},
		{
			name:            "poller run with inference worker",
			workerType:      WorkerTypeInference,
			workstationType: WorkstationTypePoller,
			wantCompatible:  false,
		},
		{
			name:            "legacy default workstation with inference worker",
			workerType:      WorkerTypeInference,
			workstationType: "",
			kind:            WorkstationKindStandard,
			wantCompatible:  true,
		},
		{
			name:            "legacy default workstation with script worker",
			workerType:      WorkerTypeScript,
			workstationType: "",
			kind:            WorkstationKindStandard,
			wantCompatible:  true,
		},
	}
}

func TestWorkerWorkstationBehaviorMismatchMessage_UsesBehaviorTerminology(t *testing.T) {
	msg := WorkerWorkstationBehaviorMismatchMessage(
		"plan-task",
		WorkstationTypeAgent,
		WorkstationKindStandard,
		"executor",
		WorkerTypeInference,
	)
	for _, term := range []string{"AGENT_RUN", "INFERENCE_WORKER", "agent-run", "inference worker"} {
		if !strings.Contains(msg, term) {
			t.Fatalf("message %q missing %q", msg, term)
		}
	}
}
