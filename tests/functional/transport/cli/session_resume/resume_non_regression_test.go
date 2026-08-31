package session_resume_test

import (
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIResumeSmokeLane_NonResumeTerminalSessionShowPreservesShippedCLIReadSemantics(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeSucceededHarness(t)
	sessionID := harness.startSucceededSession(t)

	read := readDurableSessionViaCLI(t, harness, sessionID)
	if read.SessionId != sessionID {
		t.Fatalf("sessionId = %q, want %q", read.SessionId, sessionID)
	}
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.Lifecycle != nil && read.Lifecycle.ResumedAt != nil {
		t.Fatalf("terminal non-resume read should not expose resumedAt: %#v", read.Lifecycle)
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
	}

	dispatches := readDispatchesViaHTTP(t, harness, sessionID)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("terminal simple-final dispatches = %#v, want empty", dispatches.Dispatches)
	}
}

// TestCLIResumeSmokeLane_RetiredDispatchCommandLeavesRESTReadAvailable proves resume inspection remains available after retiring the dispatch command.
func TestCLIResumeSmokeLane_RetiredDispatchCommandLeavesRESTReadAvailable(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeSucceededHarness(t)

	_, err := harness.executeCLI(t, "session", "dispatches", "session-beta")
	if err == nil || !strings.Contains(err.Error(), `unknown command "dispatches"`) {
		t.Fatalf("retired session dispatches command = %v, want unknown command", err)
	}

	workflowName := "simple-final"
	started := harness.startDurableSession(t, factoryapi.FactorySessionExecutionRequest{
		RequestId: harness.fixture.nextRequestID("retired-command"),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: &workflowName,
		},
		Args: &map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	waitForDurableSessionStatusViaCLI(
		t, harness, started.SessionId,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded, 15*time.Second,
	)

	listed := readDispatchesViaHTTP(t, harness, started.SessionId)
	if listed.SessionId != started.SessionId {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, started.SessionId)
	}
}
