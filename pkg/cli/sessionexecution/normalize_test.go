package sessionexecution_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/cli/sessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

const inlineWorkflowSource = `meta({ name: "review", version: 1 });
phase("setup");
log("starting");`

func TestNormalizeStartRequest_ResolvesFactorySyncFixtureRequest(t *testing.T) {
	got, mode, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeSync,
		RequestID: "req-petri-success-001",
		FactoryID: "customer-support-triage",
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if mode != sessionexecution.ExecutionModeSync {
		t.Fatalf("mode = %q, want sync", mode)
	}
	if got.RequestID != "req-petri-success-001" {
		t.Fatalf("requestId = %q", got.RequestID)
	}
	if got.Source.Kind != workflowsource.KindFactoryID || got.Source.FactoryID != "customer-support-triage" {
		t.Fatalf("source = %#v", got.Source)
	}
	if got.Wait == nil {
		t.Fatal("wait options = nil, want sync wait envelope")
	}
}

func TestNormalizeStartRequest_ResolvesAsyncTimeoutFixtureRequest(t *testing.T) {
	timeoutMillis := int64(30000)
	got, mode, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:              sessionexecution.ExecutionModeAsync,
		RequestID:         "req-js-timeout-001",
		WorkflowName:      "long-running-audit",
		WaitTimeoutMillis: &timeoutMillis,
		CancelOnTimeout:   true,
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if mode != sessionexecution.ExecutionModeAsync {
		t.Fatalf("mode = %q, want async", mode)
	}
	if got.Source.WorkflowName != "long-running-audit" {
		t.Fatalf("workflowName = %q", got.Source.WorkflowName)
	}
	if got.Wait != nil {
		t.Fatal("async start should not attach wait options")
	}
}

func TestNormalizeStartRequest_ResolvesIdempotentReplayFixtureRequest(t *testing.T) {
	got, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:         sessionexecution.ExecutionModeAsync,
		RequestID:    "req-idempotent-replay-001",
		WorkflowFile: ".claude/workflows/idempotent.yaml",
		ArgsJSON:     `{"task":"replay"}`,
		PolicyHash:   "req-policy-idempotent",
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if got.Args["task"] != "replay" {
		t.Fatalf("args = %#v", got.Args)
	}
	if got.RequestedPolicy["policyHash"] != "req-policy-idempotent" {
		t.Fatalf("requestedPolicy = %#v", got.RequestedPolicy)
	}
}

func TestNormalizeStartRequest_EquivalentCLIInputsProduceStableIdempotencyTuple(t *testing.T) {
	base := sessionexecution.StartConfig{
		Mode:         sessionexecution.ExecutionModeAsync,
		RequestID:    "req-idempotent-replay-001",
		WorkflowFile: ".claude/workflows/idempotent.yaml",
		ArgsJSON:     `{"task":"replay"}`,
		PolicyHash:   "req-policy-idempotent",
	}
	equivalent := base
	equivalent.WorkflowFile = "  .claude/workflows/idempotent.yaml  "
	equivalent.PolicyHash = "  req-policy-idempotent  "

	first, _, err := sessionexecution.NormalizeStartRequest(base)
	if err != nil {
		t.Fatalf("first normalize: %v", err)
	}
	second, _, err := sessionexecution.NormalizeStartRequest(equivalent)
	if err != nil {
		t.Fatalf("second normalize: %v", err)
	}

	firstHash, err := factorysessionexecution.IdempotencyTupleHash(first)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	secondHash, err := factorysessionexecution.IdempotencyTupleHash(second)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hash mismatch: %q vs %q", firstHash, secondHash)
	}
}

func TestNormalizeStartRequest_RejectsUnsupportedMode(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionMode("batch"),
		RequestID: "req-001",
		FactoryID: "customer-support-triage",
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeUnsupportedMode, "mode")
}

func TestNormalizeStartRequest_RejectsMissingMode(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		RequestID: "req-001",
		FactoryID: "customer-support-triage",
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeUnsupportedMode, "mode")
}

func TestNormalizeStartRequest_RejectsMissingSource(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeAsync,
		RequestID: "req-001",
		StdinIsTTY: func() bool {
			return true
		},
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeMissingSource, "source")
}

func TestNormalizeStartRequest_RejectsConflictingSourceSelectors(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:         sessionexecution.ExecutionModeAsync,
		RequestID:    "req-001",
		FactoryID:    "customer-support-triage",
		WorkflowName: "review",
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeSourceConflict, "source")
}

func TestNormalizeStartRequest_RejectsBadFactorySource(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeAsync,
		RequestID: "req-001",
		FactoryID: "   ",
	})
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "source.factoryId" {
		t.Fatalf("error = %v, want source.factoryId validation error", err)
	}
}

func TestNormalizeStartRequest_RejectsPositionalAndStdinInlineConflict(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:           sessionexecution.ExecutionModeAsync,
		RequestID:      "req-inline-001",
		PositionalArgs: []string{inlineWorkflowSource},
		Stdin:          strings.NewReader("meta({ name: \"stdin\" });"),
		StdinIsTTY: func() bool {
			return false
		},
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeSourceConflict, "source")
}

func TestNormalizeStartRequest_RejectsInvocationStylePositionalStdinConflict(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:           sessionexecution.ExecutionModeAsync,
		RequestID:      "req-inline-001",
		PositionalArgs: []string{"meta({ name: \"positional\" });"},
		Stdin:          strings.NewReader("meta({ name: \"stdin\" });"),
		StdinIsTTY: func() bool {
			return false
		},
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeSourceConflict, "source")
}

func TestNormalizeStartRequest_ResolvesInlineWorkflowFromStdin(t *testing.T) {
	got, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeAsync,
		RequestID: "req-inline-stdin-001",
		PositionalArgs: []string{
			"-",
		},
		Stdin: strings.NewReader(inlineWorkflowSource),
		StdinIsTTY: func() bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if got.Source.Kind != workflowsource.KindInlineWorkflow {
		t.Fatalf("source kind = %q", got.Source.Kind)
	}
	if got.Source.InlineWorkflow.InlineSource != inlineWorkflowSource {
		t.Fatalf("inline source = %q", got.Source.InlineWorkflow.InlineSource)
	}
}

func TestNormalizeStartRequest_ResolvesWorkflowFileFromPositionalPath(t *testing.T) {
	projectRoot := t.TempDir()
	workflowPath := filepath.Join(projectRoot, "review.js")
	if err := os.WriteFile(workflowPath, []byte(inlineWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	got, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:           sessionexecution.ExecutionModeAsync,
		RequestID:      "req-file-001",
		PositionalArgs: []string{workflowPath},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if got.Source.Kind != workflowsource.KindWorkflowFile || got.Source.WorkflowFile != workflowPath {
		t.Fatalf("source = %#v", got.Source)
	}
}

func TestNormalizeStartRequest_RejectsInvalidArgsJSON(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:      sessionexecution.ExecutionModeAsync,
		RequestID: "req-001",
		FactoryID: "customer-support-triage",
		ArgsJSON:  `["not","an","object"]`,
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeInvalidArgs, "args")
}

func TestNormalizeStartRequest_RejectsConflictingPolicySelectors(t *testing.T) {
	_, _, err := sessionexecution.NormalizeStartRequest(sessionexecution.StartConfig{
		Mode:       sessionexecution.ExecutionModeAsync,
		RequestID:  "req-001",
		FactoryID:  "customer-support-triage",
		PolicyJSON: `{"policyHash":"req-policy"}`,
		PolicyHash: "req-policy",
	})
	assertExecutionError(t, err, sessionexecution.ErrorCodeSourceConflict, "requestedPolicy")
}

func assertExecutionError(t *testing.T, err error, code, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", code)
	}
	var executionErr *sessionexecution.ExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("error = %T, want ExecutionError", err)
	}
	if executionErr.Code != code {
		t.Fatalf("code = %q, want %q", executionErr.Code, code)
	}
	if executionErr.Field != field {
		t.Fatalf("field = %q, want %q", executionErr.Field, field)
	}
}

func TestWriteExecutionError_JSONModeRendersDeterministicPayload(t *testing.T) {
	var output bytes.Buffer
	err := &sessionexecution.ExecutionError{
		Code:    sessionexecution.ErrorCodeUnsupportedMode,
		Message: `session execution mode "batch" is unsupported: use sync or async`,
		Field:   "mode",
	}
	if !sessionexecution.WriteExecutionError(&output, err, true) {
		t.Fatal("WriteExecutionError = false, want true")
	}

	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	}
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if payload.Code != sessionexecution.ErrorCodeUnsupportedMode {
		t.Fatalf("code = %q", payload.Code)
	}
	if payload.Field != "mode" {
		t.Fatalf("field = %q", payload.Field)
	}
	if !strings.Contains(payload.Message, "unsupported") {
		t.Fatalf("message = %q", payload.Message)
	}
}

func TestWriteExecutionError_HumanModeRendersStableLine(t *testing.T) {
	var output bytes.Buffer
	err := &sessionexecution.ExecutionError{
		Code:    sessionexecution.ErrorCodeMissingSource,
		Message: "session execution source is required",
		Field:   "source",
	}
	if !sessionexecution.WriteExecutionError(&output, err, false) {
		t.Fatal("WriteExecutionError = false, want true")
	}
	got := strings.TrimSpace(output.String())
	want := sessionexecution.ErrorCodeMissingSource + ": session execution source is required (source)"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestWriteExecutionError_MapsValidationErrors(t *testing.T) {
	var output bytes.Buffer
	err := factorysessionexecution.NewValidationError("requestId", "requestId is required")
	if !sessionexecution.WriteExecutionError(&output, err, true) {
		t.Fatal("WriteExecutionError = false, want true")
	}
	var payload struct {
		Code  string `json:"code"`
		Field string `json:"field"`
	}
	if decodeErr := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &payload); decodeErr != nil {
		t.Fatalf("decode json: %v", decodeErr)
	}
	if payload.Code != sessionexecution.ErrorCodeValidation {
		t.Fatalf("code = %q", payload.Code)
	}
	if payload.Field != "requestId" {
		t.Fatalf("field = %q", payload.Field)
	}
}

func TestWriteExecutionError_ReturnsFalseForUnknownErrors(t *testing.T) {
	if sessionexecution.WriteExecutionError(nil, errors.New("other"), true) {
		t.Fatal("WriteExecutionError = true, want false")
	}
}
