package workflowruntime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestRuntimeRecordJSON_CheckpointRoundTripPreservesTypedResumeData(t *testing.T) {
	original := workflowruntime.RuntimeRecord{
		Sequence: 7,
		Kind:     workflowruntime.RecordKindCheckpoint,
		Checkpoint: &workflowruntime.CheckpointRecord{
			ID:      "checkpoint-7",
			Label:   "after-plan",
			Summary: "resume after planning",
			State:   map[string]any{"next": "dispatch", "ordinal": float64(3)},
		},
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal checkpoint record: %v", err)
	}
	var decoded workflowruntime.RuntimeRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal checkpoint record: %v", err)
	}
	if decoded.Kind != workflowruntime.RecordKindCheckpoint || decoded.Sequence != 7 || decoded.Checkpoint == nil {
		t.Fatalf("decoded record = %#v, want typed checkpoint at sequence 7", decoded)
	}
	if decoded.Checkpoint.ID != "checkpoint-7" || decoded.Checkpoint.State["next"] != "dispatch" {
		t.Fatalf("decoded checkpoint = %#v, want original identity and resume state", decoded.Checkpoint)
	}
}

func TestRuntimeRecordJSON_RejectsUnknownAndMismatchedTypedRecords(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		message string
	}{
		{name: "unknown kind", raw: `{"sequence":1,"kind":"petri_transition","checkpoint":{"id":"checkpoint-1"}}`, message: `unsupported kind "petri_transition"`},
		{name: "missing payload", raw: `{"sequence":1,"kind":"checkpoint"}`, message: "matching payload is required"},
		{name: "foreign payload", raw: `{"sequence":1,"kind":"checkpoint","checkpoint":{"id":"checkpoint-1"},"phase":{"name":"plan"}}`, message: "unexpected phase payload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var record workflowruntime.RuntimeRecord
			err := json.Unmarshal([]byte(test.raw), &record)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want message containing %q", err, test.message)
			}
		})
	}
}

func TestRun_SimpleFinalWorkflow_ProjectsStructuredPrimaryResult(t *testing.T) {
	source := readFixture(t, "simple-final.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
		"count":   3,
		"prefix":  "you",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "simple-final.workflow.js",
		SessionID: "session-simple-final",
		Args:      args,
		Metadata: map[string]string{
			"name":        "simple-final",
			"description": "returns a structured final value",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	first := runSuccessful(t, req)
	second := runSuccessful(t, req)
	if string(first.Value.JSON) != string(second.Value.JSON) {
		t.Fatalf("value drift across runs: first=%s second=%s", first.Value.JSON, second.Value.JSON)
	}

	projected := projectPrimaryJSON(t, req.SessionID, first.Value)
	want := map[string]any{
		"label":       "simple-final",
		"description": "returns a structured final value",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
	}
	assertProjectedFields(t, projected, want)
}

func TestRun_WorkflowFinal_ProjectsStructuredPrimaryResult(t *testing.T) {
	source := readFixture(t, "workflow-final.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
		"count":   3,
		"prefix":  "you",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "workflow-final.workflow.js",
		SessionID: "session-workflow-final",
		Args:      args,
		Metadata: map[string]string{
			"name":        "workflow-final",
			"description": "completes through workflow.final",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	want := map[string]any{
		"label":       "workflow-final",
		"description": "completes through workflow.final",
		"subject":     "workflows",
		"repeat":      float64(3),
		"echo":        "you:workflows",
		"mechanism":   "workflow.final",
	}
	assertProjectedFields(t, projected, want)
}

func TestRun_SyntaxError_FailsBeforeExecution(t *testing.T) {
	source := readFixture(t, "syntax-error.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "syntax-error.workflow.js",
		SessionID: "session-syntax-error",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": "syntax-error"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runPreExecutionFailure(t, req)
	if outcome.Failure.Code != workflowruntime.CodePreExecutionInvalid {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodePreExecutionInvalid)
	}
	if !strings.Contains(outcome.Failure.Message, workflowvalidation.CodeSyntaxError) {
		t.Fatalf("failure message = %q, want validation code %q", outcome.Failure.Message, workflowvalidation.CodeSyntaxError)
	}
	if !strings.Contains(outcome.Failure.Message, "syntax-error.workflow.js") {
		t.Fatalf("failure message = %q, want source ref context", outcome.Failure.Message)
	}
}

func TestRun_ValidationFailure_FailsBeforeExecution(t *testing.T) {
	source := readFixture(t, "validation-failure.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "validation-failure.workflow.js",
		SessionID: "session-validation-failure",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": "validation-failure"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runPreExecutionFailure(t, req)
	if outcome.Failure.Code != workflowruntime.CodePreExecutionInvalid {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodePreExecutionInvalid)
	}
	if !strings.Contains(outcome.Failure.Message, workflowvalidation.CodeUnsupportedGlobal) {
		t.Fatalf("failure message = %q, want validation code %q", outcome.Failure.Message, workflowvalidation.CodeUnsupportedGlobal)
	}
	if !strings.Contains(outcome.Failure.Message, "console") {
		t.Fatalf("failure message = %q, want unsupported global context", outcome.Failure.Message)
	}
}

func TestRun_PreExecutionFailure_DoesNotInvokeHooks(t *testing.T) {
	source := readFixture(t, "syntax-error.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "syntax-error.workflow.js",
		SessionID: "session-syntax-error-hooks",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	hooksCalled := false
	hooks := workflowruntime.Hooks{
		OnResult: func(workflowresult.TypedValue) error {
			hooksCalled = true
			return nil
		},
		OnArtifact: func(string, json.RawMessage) error {
			hooksCalled = true
			return nil
		},
	}

	outcome, err := workflowruntime.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatal("expected pre-execution failure")
	}
	if hooksCalled {
		t.Fatal("result/artifact hooks were invoked for pre-execution failure")
	}
}

func TestRun_ThrowError_ReturnsStableScriptFailure(t *testing.T) {
	source := readFixture(t, "throw-error.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "throw-error.workflow.js",
		SessionID: "session-throw-error",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": "throw-error"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runExecutionFailure(t, req)
	if outcome.Failure.Code != workflowruntime.CodeScriptError {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeScriptError)
	}
	if !strings.Contains(outcome.Failure.Message, "workflow execution failed: workflows") {
		t.Fatalf("failure message = %q, want preserved JavaScript error text", outcome.Failure.Message)
	}
	assertFailureDoesNotProjectPrimaryResult(t, req.SessionID, outcome)
}

func TestRun_UnresolvedFinal_ReturnsStableFailure(t *testing.T) {
	source := readFixture(t, "unresolved-final.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "unresolved-final.workflow.js",
		SessionID: "session-unresolved-final",
		Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
		Metadata:  map[string]string{"name": "unresolved-final"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runExecutionFailure(t, req)
	if outcome.Failure.Code != workflowruntime.CodeUnresolvedFinal {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeUnresolvedFinal)
	}
	if !strings.Contains(outcome.Failure.Message, "without a returned or final value") {
		t.Fatalf("failure message = %q, want unresolved-final diagnostic", outcome.Failure.Message)
	}
	assertFailureDoesNotProjectPrimaryResult(t, req.SessionID, outcome)
}

func TestRun_InvalidTerminalValue_ReturnsStableInvalidResultFailure(t *testing.T) {
	cases := []struct {
		fixture      string
		wantCode     string
		wantFragment string
	}{
		{
			fixture:      "invalid-return-function.workflow.js",
			wantCode:     workflowresult.CodeUnsupportedType,
			wantFragment: "function value",
		},
		{
			fixture:      "invalid-unresolved-promise.workflow.js",
			wantCode:     workflowresult.CodeUnresolvedPromise,
			wantFragment: "unresolved promise",
		},
		{
			fixture:      "invalid-host-path-json.workflow.js",
			wantCode:     workflowresult.CodeArtifactURIHostPath,
			wantFragment: "/etc/passwd",
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			source := readFixture(t, tc.fixture)
			req := workflowruntime.Request{
				Source:    source,
				SourceRef: tc.fixture,
				SessionID: "session-" + strings.TrimSuffix(tc.fixture, ".workflow.js"),
				Args:      marshalArgs(t, map[string]any{}),
				Metadata:  map[string]string{"name": strings.TrimSuffix(tc.fixture, ".workflow.js")},
				Policy:    workflowpolicy.DefaultEffectivePolicy(),
			}

			outcome := runInvalidResultFailure(t, req)
			if outcome.Failure.Code != workflowruntime.CodeInvalidResult {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeInvalidResult)
			}
			if !strings.Contains(outcome.Failure.Message, tc.wantCode) {
				t.Fatalf("failure message = %q, want validation code %q", outcome.Failure.Message, tc.wantCode)
			}
			if !strings.Contains(outcome.Failure.Message, tc.wantFragment) {
				t.Fatalf("failure message = %q, want fragment %q", outcome.Failure.Message, tc.wantFragment)
			}
			assertFailureDoesNotProjectPrimaryResult(t, req.SessionID, outcome)
		})
	}
}

func TestRun_InvalidTerminalValue_DoesNotInvokeHooks(t *testing.T) {
	source := readFixture(t, "invalid-return-function.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "invalid-return-function.workflow.js",
		SessionID: "session-invalid-result-hooks",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	hooksCalled := false
	hooks := workflowruntime.Hooks{
		OnResult: func(workflowresult.TypedValue) error {
			hooksCalled = true
			return nil
		},
		OnArtifact: func(string, json.RawMessage) error {
			hooksCalled = true
			return nil
		},
	}

	outcome, err := workflowruntime.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatal("expected invalid-result failure")
	}
	if outcome.Failure.Code != workflowruntime.CodeInvalidResult {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeInvalidResult)
	}
	if hooksCalled {
		t.Fatal("result/artifact hooks were invoked for invalid terminal value")
	}
}

func TestRun_ExecutionFailure_DoesNotInvokeHooks(t *testing.T) {
	source := readFixture(t, "throw-error.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "throw-error.workflow.js",
		SessionID: "session-throw-error-hooks",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	hooksCalled := false
	hooks := workflowruntime.Hooks{
		OnResult: func(workflowresult.TypedValue) error {
			hooksCalled = true
			return nil
		},
		OnArtifact: func(string, json.RawMessage) error {
			hooksCalled = true
			return nil
		},
	}

	outcome, err := workflowruntime.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatal("expected execution failure")
	}
	if hooksCalled {
		t.Fatal("result/artifact hooks were invoked for execution failure")
	}
}

func TestRun_CanceledContext_ReturnsCanceledFailure(t *testing.T) {
	req := busyLoopRequest(t, "session-canceled")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan workflowruntime.Outcome, 1)
	errCh := make(chan error, 1)
	go func() {
		outcome, err := workflowruntime.Run(ctx, req, workflowruntime.Hooks{})
		if err != nil {
			errCh <- err
			return
		}
		done <- outcome
	}()

	cancel()

	select {
	case err := <-errCh:
		t.Fatalf("Run() error = %v", err)
	case outcome := <-done:
		assertCanceledFailure(t, req.SessionID, outcome)
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after cancellation within bounded wait")
	}
}

func TestRun_AlreadyCanceledContext_ReturnsCanceledFailure(t *testing.T) {
	req := busyLoopRequest(t, "session-already-canceled")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	outcome, err := workflowruntime.Run(ctx, req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	assertCanceledFailure(t, req.SessionID, outcome)
}

func TestRun_Timeout_ReturnsTimeoutFailure(t *testing.T) {
	req := busyLoopRequest(t, "session-timeout")

	outcome, err := workflowruntime.RunWithTimeout(context.Background(), 50*time.Millisecond, req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("RunWithTimeout() error = %v", err)
	}
	assertTimeoutFailure(t, req.SessionID, outcome)
}

func TestRun_CancelOrTimeout_DoesNotInvokeHooks(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		req := busyLoopRequest(t, "session-canceled-hooks")
		ctx, cancel := context.WithCancel(context.Background())
		hooksCalled := false
		hooks := workflowruntime.Hooks{
			OnResult: func(workflowresult.TypedValue) error {
				hooksCalled = true
				return nil
			},
			OnArtifact: func(string, json.RawMessage) error {
				hooksCalled = true
				return nil
			},
		}

		done := make(chan workflowruntime.Outcome, 1)
		errCh := make(chan error, 1)
		go func() {
			outcome, err := workflowruntime.Run(ctx, req, hooks)
			if err != nil {
				errCh <- err
				return
			}
			done <- outcome
		}()
		cancel()

		select {
		case err := <-errCh:
			t.Fatalf("Run() error = %v", err)
		case outcome := <-done:
			if outcome.OK {
				t.Fatal("expected canceled failure")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Run() did not return after cancellation within bounded wait")
		}
		if hooksCalled {
			t.Fatal("result/artifact hooks were invoked after cancellation")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		req := busyLoopRequest(t, "session-timeout-hooks")
		hooksCalled := false
		hooks := workflowruntime.Hooks{
			OnResult: func(workflowresult.TypedValue) error {
				hooksCalled = true
				return nil
			},
			OnArtifact: func(string, json.RawMessage) error {
				hooksCalled = true
				return nil
			},
		}

		outcome, err := workflowruntime.RunWithTimeout(context.Background(), 50*time.Millisecond, req, hooks)
		if err != nil {
			t.Fatalf("RunWithTimeout() error = %v", err)
		}
		if outcome.OK {
			t.Fatal("expected timeout failure")
		}
		if hooksCalled {
			t.Fatal("result/artifact hooks were invoked after timeout")
		}
	})
}

func TestRun_ForbiddenHostAccess_ReturnsDeniedCapabilityFailure(t *testing.T) {
	cases := []struct {
		fixture string
		keyword string
	}{
		{fixture: "host-access-fs.workflow.js", keyword: "require"},
		{fixture: "host-access-process.workflow.js", keyword: "process"},
		{fixture: "host-access-network.workflow.js", keyword: "network"},
		{fixture: "host-access-shell.workflow.js", keyword: "process"},
		{fixture: "host-access-import.workflow.js", keyword: "import"},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			source := readFixture(t, tc.fixture)
			req := workflowruntime.Request{
				Source:    source,
				SourceRef: tc.fixture,
				SessionID: "session-" + strings.TrimSuffix(tc.fixture, ".workflow.js"),
				Args:      marshalArgs(t, map[string]any{"subject": "workflows"}),
				Metadata:  map[string]string{"name": strings.TrimSuffix(tc.fixture, ".workflow.js")},
				Policy:    workflowpolicy.DefaultEffectivePolicy(),
			}

			outcome := runHostAccessFailure(t, req)
			if outcome.Failure.Code != workflowruntime.CodeDeniedCapability {
				t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeDeniedCapability)
			}
			if !strings.Contains(outcome.Failure.Message, workflowvalidation.CodeForbiddenHostAccess) {
				t.Fatalf("failure message = %q, want validation code %q", outcome.Failure.Message, workflowvalidation.CodeForbiddenHostAccess)
			}
			if !strings.Contains(strings.ToLower(outcome.Failure.Message), tc.keyword) {
				t.Fatalf("failure message = %q, want host-access context %q", outcome.Failure.Message, tc.keyword)
			}
			if !strings.Contains(outcome.Failure.Message, tc.fixture) {
				t.Fatalf("failure message = %q, want source ref context", outcome.Failure.Message)
			}
			assertFailureDoesNotProjectPrimaryResult(t, req.SessionID, outcome)
		})
	}
}

func TestRun_ForbiddenHostAccess_DoesNotInvokeHooks(t *testing.T) {
	source := readFixture(t, "host-access-fs.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "host-access-fs.workflow.js",
		SessionID: "session-host-access-hooks",
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
	hooksCalled := false
	hooks := workflowruntime.Hooks{
		OnResult: func(workflowresult.TypedValue) error {
			hooksCalled = true
			return nil
		},
		OnArtifact: func(string, json.RawMessage) error {
			hooksCalled = true
			return nil
		},
	}

	outcome, err := workflowruntime.Run(context.Background(), req, hooks)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatal("expected host-access denial")
	}
	if outcome.Failure.Code != workflowruntime.CodeDeniedCapability {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeDeniedCapability)
	}
	if hooksCalled {
		t.Fatal("result/artifact hooks were invoked for forbidden host access")
	}
}

func TestRun_WorkflowFinalAndReturn_PrefersWorkflowFinal(t *testing.T) {
	source := readFixture(t, "workflow-final-and-return.workflow.js")
	args := marshalArgs(t, map[string]any{
		"subject": "workflows",
	})

	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "workflow-final-and-return.workflow.js",
		SessionID: "session-workflow-final-and-return",
		Args:      args,
		Metadata: map[string]string{
			"name": "workflow-final-and-return",
		},
		Policy: workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	assertProjectedFields(t, projected, map[string]any{
		"label":     "workflow-final-and-return",
		"mechanism": "workflow.final",
		"subject":   "workflows",
	})
	if projected["mechanism"] == "return" {
		t.Fatalf("projected mechanism = return, want workflow.final precedence")
	}
}

func busyLoopRequest(t *testing.T, sessionID string) workflowruntime.Request {
	t.Helper()
	return workflowruntime.Request{
		Source:    readFixture(t, "busy-loop.workflow.js"),
		SourceRef: "busy-loop.workflow.js",
		SessionID: sessionID,
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}
}

func assertCanceledFailure(t *testing.T, sessionID string, outcome workflowruntime.Outcome) {
	t.Helper()
	if outcome.OK {
		t.Fatal("expected canceled failure")
	}
	if outcome.Failure.Code != workflowruntime.CodeCanceled {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeCanceled)
	}
	assertFailureDoesNotProjectPrimaryResult(t, sessionID, outcome)
}

func assertTimeoutFailure(t *testing.T, sessionID string, outcome workflowruntime.Outcome) {
	t.Helper()
	if outcome.OK {
		t.Fatal("expected timeout failure")
	}
	if outcome.Failure.Code != workflowruntime.CodeTimeout {
		t.Fatalf("failure code = %q, want %q", outcome.Failure.Code, workflowruntime.CodeTimeout)
	}
	if !strings.Contains(strings.ToLower(outcome.Failure.Message), "deadline exceeded") &&
		!strings.Contains(strings.ToLower(outcome.Failure.Message), "timed out") {
		t.Fatalf("failure message = %q, want timeout diagnostic", outcome.Failure.Message)
	}
	assertFailureDoesNotProjectPrimaryResult(t, sessionID, outcome)
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(content)
}

func marshalArgs(t *testing.T, args map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	return raw
}

func runInvalidResultFailure(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatalf("Run() expected invalid-result failure, got success value = %#v", outcome.Value)
	}
	if outcome.Failure.Code == "" {
		t.Fatalf("Run() missing failure diagnostic: %#v", outcome)
	}
	return outcome
}

func runExecutionFailure(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatalf("Run() expected execution failure, got success value = %#v", outcome.Value)
	}
	if outcome.Failure.Code == "" {
		t.Fatalf("Run() missing failure diagnostic: %#v", outcome)
	}
	return outcome
}

func assertFailureDoesNotProjectPrimaryResult(t *testing.T, sessionID string, outcome workflowruntime.Outcome) {
	t.Helper()
	if outcome.OK {
		t.Fatal("expected failure outcome")
	}
	parts, projection := workflowresult.ProjectPrimaryResult(sessionID, outcome.Value, nil)
	if !projection.HasIssues() && len(parts) > 0 {
		t.Fatalf("failure outcome projected primary result parts=%#v", parts)
	}
}

func runHostAccessFailure(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatalf("Run() expected host-access denial, got success value = %#v", outcome.Value)
	}
	if outcome.Failure.Code == "" {
		t.Fatalf("Run() missing failure diagnostic: %#v", outcome)
	}
	return outcome
}

func runPreExecutionFailure(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.OK {
		t.Fatalf("Run() expected pre-execution failure, got success value = %#v", outcome.Value)
	}
	if outcome.Failure.Code == "" {
		t.Fatalf("Run() missing failure diagnostic: %#v", outcome)
	}
	return outcome
}

func runSuccessful(t *testing.T, req workflowruntime.Request) workflowruntime.Outcome {
	t.Helper()
	outcome, err := workflowruntime.Run(t.Context(), req, workflowruntime.Hooks{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !outcome.OK {
		t.Fatalf("Run() failure = %#v", outcome.Failure)
	}
	validation := workflowresult.ValidateTypedValue(outcome.Value)
	if validation.HasIssues() {
		t.Fatalf("result validation = %#v", validation.Issues)
	}
	return outcome
}

func projectPrimaryJSON(t *testing.T, sessionID string, value workflowresult.TypedValue) map[string]any {
	t.Helper()
	parts, projection := workflowresult.ProjectPrimaryResult(sessionID, value, nil)
	if projection.HasIssues() {
		t.Fatalf("primary projection validation = %#v", projection.Issues)
	}
	if len(parts) != 1 || parts[0].Type != work.WorkContentPartTypeJSON {
		t.Fatalf("parts = %#v", parts)
	}
	var projected map[string]any
	if err := json.Unmarshal(parts[0].JSON, &projected); err != nil {
		t.Fatalf("unmarshal projected json: %v", err)
	}
	generated := contentcontract.GeneratedPtrFromParts(parts)
	if generated == nil {
		t.Fatal("expected generated primary result content")
	}
	return projected
}

func assertProjectedFields(t *testing.T, projected map[string]any, want map[string]any) {
	t.Helper()
	for key, wantValue := range want {
		if projected[key] != wantValue {
			t.Fatalf("projected[%q] = %#v, want %#v", key, projected[key], wantValue)
		}
	}
}
func TestResumingChildExecutor_ReplaysCompletedDispatchWithoutCallingBase(t *testing.T) {
	base := &countingChildExecutor{}
	resume := workflowruntime.ResumeContext{
		CompletedDispatchIDs: []string{"dispatch-1"},
		CompletedChildResults: map[string]workflowruntime.ChildExecutionResult{
			"dispatch-1": {
				DispatchID:    "dispatch-1",
				ChildIndex:    1,
				Status:        workflowruntime.ChildDispatchStatusCompleted,
				ExecutionMode: workflowruntime.ChildExecutionModeFake,
				Output: map[string]any{
					"text": "cached:first",
				},
			},
		},
	}
	executor := workflowruntime.NewResumingChildExecutor(base, resume)

	first, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{Label: "step-one"})
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	if first.Output["text"] != "cached:first" {
		t.Fatalf("first output = %#v, want cached:first", first.Output)
	}
	if base.calls != 0 {
		t.Fatalf("base calls after replay = %d, want 0", base.calls)
	}

	second, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{Label: "step-two"})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.DispatchID != "dispatch-2" {
		t.Fatalf("second dispatchId = %q, want dispatch-2", second.DispatchID)
	}
	if base.calls != 1 {
		t.Fatalf("base calls after second dispatch = %d, want 1", base.calls)
	}
}

func TestRun_ResumeStateIsUndefinedOnFreshRun(t *testing.T) {
	source := readFixture(t, "resumable-checkpoint-state-branch.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "resumable-checkpoint-state-branch.workflow.js",
		SessionID: "session-resume-state-fresh",
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": "resumable-checkpoint-state-branch"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	if projected["path"] != "fresh" {
		t.Fatalf("projected path = %#v, want fresh", projected["path"])
	}
}

func TestRun_ResumeStateRehydratesCheckpointFactsForControlFlow(t *testing.T) {
	source := readFixture(t, "resumable-checkpoint-state-branch.workflow.js")
	req := workflowruntime.Request{
		Source:    source,
		SourceRef: "resumable-checkpoint-state-branch.workflow.js",
		SessionID: "session-resume-state-rehydrated",
		Args:      marshalArgs(t, map[string]any{}),
		Metadata:  map[string]string{"name": "resumable-checkpoint-state-branch"},
		Policy:    workflowpolicy.DefaultEffectivePolicy(),
		Resume: &workflowruntime.ResumeContext{
			CompletedDispatchIDs: []string{"dispatch-1"},
			CompletedChildResults: map[string]workflowruntime.ChildExecutionResult{
				"dispatch-1": {
					DispatchID:    "dispatch-1",
					ChildIndex:    1,
					Status:        workflowruntime.ChildDispatchStatusCompleted,
					ExecutionMode: workflowruntime.ChildExecutionModeFake,
					Output: map[string]any{
						"text":  "cached:first",
						"label": "step-one",
					},
				},
			},
			CheckpointState: map[string]any{
				"step":       float64(1),
				"firstLabel": "step-one",
			},
		},
	}

	outcome := runSuccessful(t, req)
	projected := projectPrimaryJSON(t, req.SessionID, outcome.Value)
	if projected["path"] != "from-checkpoint" {
		t.Fatalf("projected path = %#v, want from-checkpoint", projected["path"])
	}
	if projected["step"] != float64(1) {
		t.Fatalf("projected step = %#v, want 1", projected["step"])
	}
	if projected["firstLabel"] != "step-one" {
		t.Fatalf("projected firstLabel = %#v, want step-one", projected["firstLabel"])
	}
}

func TestResumingChildExecutor_StartsAtNextPendingDispatchWhenCheckpointStatePresent(t *testing.T) {
	base := &countingChildExecutor{}
	resume := workflowruntime.ResumeContext{
		CompletedDispatchIDs: []string{"dispatch-1"},
		CompletedChildResults: map[string]workflowruntime.ChildExecutionResult{
			"dispatch-1": {
				DispatchID: "dispatch-1",
				Status:     workflowruntime.ChildDispatchStatusCompleted,
				Output:     map[string]any{"text": "cached:first"},
			},
		},
		CheckpointState: map[string]any{"step": float64(1)},
	}
	executor := workflowruntime.NewResumingChildExecutor(base, resume)

	result, err := executor.Execute(context.Background(), workflowruntime.ChildExecutionRequest{Label: "step-two"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.DispatchID != "dispatch-2" {
		t.Fatalf("dispatchId = %q, want dispatch-2", result.DispatchID)
	}
	if base.calls != 1 {
		t.Fatalf("base calls = %d, want 1 fresh pending dispatch", base.calls)
	}
}

type countingChildExecutor struct {
	calls int
}

func (c *countingChildExecutor) Execute(_ context.Context, req workflowruntime.ChildExecutionRequest) (workflowruntime.ChildExecutionResult, error) {
	c.calls++
	dispatchID := "dispatch-2"
	if req.ReservedIdentity != nil && req.ReservedIdentity.DispatchID != "" {
		dispatchID = req.ReservedIdentity.DispatchID
	}
	return workflowruntime.ChildExecutionResult{
		DispatchID:    dispatchID,
		ChildIndex:    2,
		Status:        workflowruntime.ChildDispatchStatusCompleted,
		ExecutionMode: workflowruntime.ChildExecutionModeFake,
		Output: map[string]any{
			"text":  "fresh:second",
			"label": req.Label,
		},
	}, nil
}

func TestCompletedChildResultsFromRecords_RestoresStoredLiveProviderOutput(t *testing.T) {
	storedOutput := map[string]any{
		"text":  "live:resumable:step-one",
		"label": "step-one",
	}
	records := []workflowruntime.RuntimeRecord{
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:    "dispatch-1",
				ChildIndex:    1,
				Status:        workflowruntime.ChildDispatchStatusCompleted,
				Label:         "step-one",
				ExecutionMode: workflowruntime.ChildExecutionModeLive,
				Output:        storedOutput,
			},
		},
	}

	completed := workflowruntime.CompletedChildResultsFromRecords(records)
	result, ok := completed["dispatch-1"]
	if !ok {
		t.Fatal("expected completed child result for dispatch-1")
	}
	if result.Output["text"] != "live:resumable:step-one" {
		t.Fatalf("restored output text = %#v, want live:resumable:step-one", result.Output["text"])
	}
	if result.ExecutionMode != workflowruntime.ChildExecutionModeLive {
		t.Fatalf("executionMode = %q, want live-provider", result.ExecutionMode)
	}
}
