package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
)

func TestParseScriptPollerStdoutRejectsMalformedShapes(t *testing.T) {
	t.Parallel()

	rawEventJSON, err := json.Marshal(map[string]any{
		"events": []map[string]any{{"type": "WORK_REQUEST"}},
	})
	if err != nil {
		t.Fatalf("marshal raw event payload: %v", err)
	}

	tests := []struct {
		name             string
		stdout           []byte
		wantHasRequest   bool
		wantErrSubstring string
	}{
		{
			name:             "non-json stdout",
			stdout:           []byte("submitted work\n"),
			wantHasRequest:   true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "malformed work request json in request envelope",
			stdout:           []byte(`{"request":{"requestId":`),
			wantHasRequest:   true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "malformed bare work request json",
			stdout:           []byte(`{"requestId":"x","type":`),
			wantHasRequest:   true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "unsupported raw factory events",
			stdout:           rawEventJSON,
			wantHasRequest:   true,
			wantErrSubstring: "unsupported raw factory events",
		},
		{
			name:             "mixed request and submissions",
			stdout:           []byte(`{"request":{"requestId":"a","type":"FACTORY_REQUEST_BATCH","works":[]},"submissions":[]}`),
			wantHasRequest:   true,
			wantErrSubstring: "either request or submissions",
		},
		{
			name:             "unsupported work request type",
			stdout:           []byte(`{"requestId":"x","type":"UNSUPPORTED","works":[]}`),
			wantHasRequest:   true,
			wantErrSubstring: "unsupported work request type",
		},
		{
			name:             "missing request id",
			stdout:           []byte(`{"requestId":"","type":"FACTORY_REQUEST_BATCH","works":[{"name":"w","workTypeName":"task"}]}`),
			wantHasRequest:   true,
			wantErrSubstring: "requestId",
		},
		{
			name:             "malformed submissions decode",
			stdout:           []byte(`{"submissions":[{"requestId":`),
			wantHasRequest:   true,
			wantErrSubstring: "malformed stdout",
		},
		{
			name:             "empty submissions array",
			stdout:           []byte(`{"submissions":[]}`),
			wantHasRequest:   true,
			wantErrSubstring: "submissions must contain at least one item",
		},
		{
			name:             "submissions missing shared request id",
			stdout:           []byte(`{"submissions":[{"requestId":"","workId":"w1","name":"w","workTypeName":"task"}]}`),
			wantHasRequest:   true,
			wantErrSubstring: "requestId",
		},
		{
			name:             "checkpoint without cursor",
			stdout:           []byte(`{"requestId":"checkpoint-only","type":"FACTORY_REQUEST_BATCH","works":[],"checkpoint":"checkpoint-only"}`),
			wantHasRequest:   true,
			wantErrSubstring: "checkpoint requires cursor",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := parseScriptPollerStdout(tc.stdout)
			if parsed.hasRequest != tc.wantHasRequest {
				t.Fatalf("hasRequest = %v, want %v", parsed.hasRequest, tc.wantHasRequest)
			}
			if err == nil {
				t.Fatal("expected malformed-output parse error")
			}
			if !strings.Contains(err.Error(), tc.wantErrSubstring) {
				t.Fatalf("parse error = %v, want substring %q", err, tc.wantErrSubstring)
			}
		})
	}
}

func TestParseScriptPollerStdoutEmptyIsNotOutput(t *testing.T) {
	t.Parallel()

	parsed, err := parseScriptPollerStdout(nil)
	if parsed.hasRequest || err != nil {
		t.Fatalf("empty stdout = hasRequest %v err %v, want no output", parsed.hasRequest, err)
	}
}

func TestParseScriptPollerStdoutExtractsOpaqueRecoveryFacts(t *testing.T) {
	t.Parallel()

	parsed, err := parseScriptPollerStdout([]byte(`{
		"requestId":"linear-issue-batch-cursor",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"issue-cursor","workTypeName":"task"}],
		"cursor":"opaque-cursor-9",
		"checkpoint":"checkpoint-9"
	}`))
	if err != nil {
		t.Fatalf("parseScriptPollerStdout() error = %v", err)
	}
	if !parsed.hasRequest || !parsed.advancesPosition {
		t.Fatalf("parsed = %+v, want request and position advancement", parsed)
	}
	if parsed.advancedCursor != "opaque-cursor-9" || parsed.checkpoint != "checkpoint-9" {
		t.Fatalf("parsed recovery = cursor %q checkpoint %q", parsed.advancedCursor, parsed.checkpoint)
	}
}

func TestSourceIDForWorkstation(t *testing.T) {
	t.Parallel()

	if got := sourceIDForWorkstation("linear-ingress"); got != "script-poller:linear-ingress" {
		t.Fatalf("sourceIDForWorkstation() = %q, want script-poller:linear-ingress", got)
	}
	if got := sourceIDForWorkstation(" "); got != "" {
		t.Fatalf("sourceIDForWorkstation() = %q, want empty for blank workstation", got)
	}
}

func TestStableInstanceIDIsDeterministicAndNamespaced(t *testing.T) {
	t.Parallel()

	first := stableInstanceID("workflow-a", "script-poller:ingress")
	second := stableInstanceID("workflow-a", "script-poller:ingress")
	if first != second {
		t.Fatalf("stableInstanceID() = %q and %q, want deterministic value", first, second)
	}
	if !strings.HasPrefix(first, scriptPollerInstanceIDPrefix+":") {
		t.Fatalf("stableInstanceID() = %q, want script-poller namespace", first)
	}
	if strings.HasPrefix(first, "automation-instance:") {
		t.Fatalf("stableInstanceID() = %q, want script-poller namespace", first)
	}
}

func TestSupervisionForWiresAutomationSourceAndInstanceIdentity(t *testing.T) {
	t.Parallel()

	supervision := supervisionFor("workflow-cursor", "linear-ingress")
	if supervision.automationID != "workflow-cursor" {
		t.Fatalf("automationID = %q, want workflow-cursor", supervision.automationID)
	}
	if supervision.sourceID != "script-poller:linear-ingress" {
		t.Fatalf("sourceID = %q, want script-poller:linear-ingress", supervision.sourceID)
	}
	if supervision.instanceID == "" || !strings.HasPrefix(supervision.instanceID, scriptPollerInstanceIDPrefix+":") {
		t.Fatalf("instanceID = %q, want script-poller instance identity", supervision.instanceID)
	}
}

func TestMemoryCursorRecorderCommitsAndReadsOpaqueFacts(t *testing.T) {
	t.Parallel()

	recorder := newMemoryCursorRecorder()
	ctx := context.Background()
	const (
		automationID = "workflow-cursor"
		instanceID   = "instance-cursor-1"
	)

	if err := recorder.CommitCursor(ctx, commitCursorRequest{
		automationID: automationID,
		instanceID:   instanceID,
		cursor:       "opaque-cursor-1",
		checkpoint:   "checkpoint-1",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	cursor, err := recorder.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "opaque-cursor-1",
	})
	if err != nil {
		t.Fatalf("GetCursor() error = %v", err)
	}
	if cursor.AutomationID != automationID ||
		cursor.InstanceID != instanceID ||
		cursor.Cursor != "opaque-cursor-1" ||
		cursor.Checkpoint != "checkpoint-1" {
		t.Fatalf("GetCursor() = %+v, want committed opaque facts", cursor)
	}
}

func TestMemoryCursorRecorderRejectsStaleExpectedCursor(t *testing.T) {
	t.Parallel()

	recorder := newMemoryCursorRecorder()
	ctx := context.Background()
	const instanceID = "instance-stale"
	if err := recorder.CommitCursor(ctx, commitCursorRequest{
		automationID: "workflow-stale",
		instanceID:   instanceID,
		cursor:       "cursor-current",
	}); err != nil {
		t.Fatalf("CommitCursor() error = %v", err)
	}

	_, err := recorder.GetCursor(ctx, automations.GetCursorRequest{
		InstanceID:     instanceID,
		ExpectedCursor: "cursor-stale",
	})
	assertAutomationsConflict(t, err, getCursorOperation)

	err = recorder.CommitCursor(ctx, commitCursorRequest{
		automationID:   "workflow-stale",
		instanceID:     instanceID,
		expectedCursor: "cursor-stale",
		cursor:         "cursor-next",
	})
	assertAutomationsConflict(t, err, commitCursorOperation)

	cursor, err := recorder.GetCursor(ctx, automations.GetCursorRequest{InstanceID: instanceID})
	if err != nil {
		t.Fatalf("GetCursor() after stale commit = %v", err)
	}
	if cursor.Cursor != "cursor-current" {
		t.Fatalf("cursor after stale commit = %q, want authoritative cursor-current", cursor.Cursor)
	}
}

func TestSubmitFailedErrorWrapsAdmissionFailure(t *testing.T) {
	t.Parallel()

	cause := errors.New("ingress unavailable")
	err := submitFailedError(cause)
	if err == nil {
		t.Fatal("expected typed submit failure")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeFailed {
		t.Fatalf("error code = %q, want %q", typed.Code, automations.ErrorCodeFailed)
	}
	if !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("error = %v, want submit failed message", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped cause %v", err, cause)
	}
}

func TestSubmitFailedErrorNilReturnsNil(t *testing.T) {
	t.Parallel()

	if err := submitFailedError(nil); err != nil {
		t.Fatalf("submitFailedError(nil) = %v, want nil", err)
	}
}
