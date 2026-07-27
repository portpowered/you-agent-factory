package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
)

func TestDurableStartAsyncIdempotentReplayReturnsStableSessionIdentity(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	request := factorysessions.DurableStartRequest(idempotentAsyncStartRequest())
	first, err := owner.StartAsync(context.Background(), request)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	second, err := owner.StartAsync(context.Background(), request)
	if err != nil {
		t.Fatalf("replay StartAsync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if first.SessionID != "dur-sess-idempotent-001" {
		t.Fatalf("sessionId = %q, want dur-sess-idempotent-001", first.SessionID)
	}
}

func TestDurableStartSyncIdempotentReplayReturnsStableSessionIdentity(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	request := factorysessions.DurableStartRequest(startRequestForPublished(row))

	first, err := owner.StartSync(context.Background(), request)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	second, err := owner.StartSync(context.Background(), request)
	if err != nil {
		t.Fatalf("replay StartSync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if first.SessionID != row.SessionID {
		t.Fatalf("sessionId = %q, want %q", first.SessionID, row.SessionID)
	}
}

func TestDurableStartRequestIDConflictOnTupleMismatch(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	request := factorysessions.DurableStartRequest(idempotentAsyncStartRequest())
	if _, err := owner.StartAsync(context.Background(), request); err != nil {
		t.Fatalf("seed StartAsync: %v", err)
	}

	conflict := request
	conflict.Args = map[string]any{"task": "different"}
	_, err = owner.StartAsync(context.Background(), conflict)
	if !errors.Is(err, factorysessions.ErrExecutionRequestIDConflict) {
		t.Fatalf("conflicting StartAsync error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestDurableStartRequestIDConflictOnAsyncSyncModeMismatch(t *testing.T) {
	t.Parallel()

	owner, err := New(newFixtureBackedExecution(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	row := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	request := factorysessions.DurableStartRequest(startRequestForPublished(row))
	if _, err := owner.StartAsync(context.Background(), request); err != nil {
		t.Fatalf("seed StartAsync: %v", err)
	}

	_, err = owner.StartSync(context.Background(), request)
	if !errors.Is(err, factorysessions.ErrExecutionRequestIDConflict) {
		t.Fatalf("async replay StartSync error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func idempotentAsyncStartRequest() factorysessions.StartRequest {
	return factorysessions.StartRequest{
		RequestID: "req-idempotent-replay-001",
		Source: factorysessions.Source{
			Kind:         factory.WorkflowSourceKindWorkflowFile,
			WorkflowFile: ".claude/workflows/idempotent.yaml",
		},
		Args: map[string]any{"task": "replay"},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-idempotent",
		},
	}
}
