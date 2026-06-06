package executor

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestInferenceRequestForExecutionRequest_DefaultWorkingDirectoryDoesNotRequireRunnerCapability(t *testing.T) {
	req := testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-default-workdir",
			TransitionID:    "t-default-workdir",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("System prompt", "Review"),
		withAgentWorkingDirectory("/tmp/runtime-session"),
	)

	got := inferenceRequestForExecutionRequest(req, &interfaces.WorkerConfig{
		Model:         "gemini-1.5-pro",
		ModelProvider: interfaces.RunnerIDGemini,
	}, nil)

	for _, capability := range got.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityWorkingDirectory {
			t.Fatalf("capabilities = %#v, want default working directory omitted", got.RequiredOptionalCapabilities)
		}
	}
	if got.WorkingDirectory != "/tmp/runtime-session" {
		t.Fatalf("working directory = %q, want forwarded runtime path", got.WorkingDirectory)
	}
}

func TestInferenceRequestForExecutionRequest_AuthoredWorkingDirectoryRequiresRunnerCapability(t *testing.T) {
	req := testAgentRequest(
		interfaces.WorkDispatch{
			DispatchID:      "d-authored-workdir",
			TransitionID:    "t-authored-workdir",
			WorkerType:      "worker-a",
			WorkstationName: "review",
		},
		withAgentPrompts("System prompt", "Review"),
		withAgentWorkingDirectory("/tmp/authored"),
		func(req *interfaces.WorkstationExecutionRequest) {
			req.WorkingDirectoryAuthored = true
		},
	)

	got := inferenceRequestForExecutionRequest(req, &interfaces.WorkerConfig{
		Model:         "gemini-1.5-pro",
		ModelProvider: interfaces.RunnerIDGemini,
	}, nil)

	found := false
	for _, capability := range got.RequiredOptionalCapabilities {
		if capability == interfaces.RunnerOptionalCapabilityWorkingDirectory {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("capabilities = %#v, want authored working directory capability", got.RequiredOptionalCapabilities)
	}
}
