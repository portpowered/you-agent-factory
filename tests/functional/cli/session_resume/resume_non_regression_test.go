package session_resume_test

import (
	"bytes"
	"encoding/json"
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

	dispatches := readDispatchesViaCLI(t, harness, sessionID)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("terminal simple-final dispatches = %#v, want empty", dispatches.Dispatches)
	}
}

func TestCLIResumeSmokeLane_ResumeInspectionStaysOnSharedSessionHTTPSurface(t *testing.T) {
	t.Parallel()

	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	serverURL, process := startRootCLIResumeAPIServer(t, projectRoot, nil)
	harness := &cliResumeSmokeHarness{serverURL: serverURL, projectRoot: projectRoot, process: process}

	_, err := harness.executeCLI(t, "session", "dispatches", "session-beta")
	if err == nil || !strings.Contains(err.Error(), "dur-sess-*") {
		t.Fatalf("dispatches on live session id = %v, want durable-session validation error", err)
	}

	workflowName := "simple-final"
	started := startDurableSessionViaHTTP(t, serverURL, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-cli-resume-scope-001",
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

	out, err := harness.executeCLI(t, "session", "dispatches", started.SessionId)
	if err != nil {
		t.Fatalf("session dispatches durable HTTP: %v", err)
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &listed); err != nil {
		t.Fatalf("decode dispatches JSON: %v\n%s", err, out.String())
	}
	if listed.SessionId != started.SessionId {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, started.SessionId)
	}
}
