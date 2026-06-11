package workflowruntime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

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
		"label":       "workflow-final-and-return",
		"mechanism":   "workflow.final",
		"subject":     "workflows",
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
	if len(parts) != 1 || parts[0].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("parts = %#v", parts)
	}
	var projected map[string]any
	if err := json.Unmarshal(parts[0].JSON, &projected); err != nil {
		t.Fatalf("unmarshal projected json: %v", err)
	}
	generated := workcontent.GeneratedPtrFromParts(parts)
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
