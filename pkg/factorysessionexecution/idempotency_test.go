package factorysessionexecution_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

func baseRequest() factorysessionexecution.StartRequest {
	return factorysessionexecution.StartRequest{
		RequestID: "req-idempotent",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/idempotent.yaml",
		},
		Args: map[string]any{"task": "replay"},
		RequestedPolicy: map[string]any{
			"policyHash": "req-policy-idempotent",
		},
	}
}

func TestIdempotencyTupleHash_StableForEquivalentRequests(t *testing.T) {
	first, err := factorysessionexecution.NormalizeStartRequest(baseRequest())
	if err != nil {
		t.Fatalf("normalize first: %v", err)
	}
	second, err := factorysessionexecution.NormalizeStartRequest(baseRequest())
	if err != nil {
		t.Fatalf("normalize second: %v", err)
	}

	firstHash, err := factorysessionexecution.IdempotencyTupleHash(first)
	if err != nil {
		t.Fatalf("hash first: %v", err)
	}
	secondHash, err := factorysessionexecution.IdempotencyTupleHash(second)
	if err != nil {
		t.Fatalf("hash second: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hash mismatch: %q vs %q", firstHash, secondHash)
	}
}

func TestIdempotencyTupleHash_ChangesWhenArgsChange(t *testing.T) {
	first, err := factorysessionexecution.NormalizeStartRequest(baseRequest())
	if err != nil {
		t.Fatalf("normalize first: %v", err)
	}
	changed := baseRequest()
	changed.Args = map[string]any{"task": "different"}
	second, err := factorysessionexecution.NormalizeStartRequest(changed)
	if err != nil {
		t.Fatalf("normalize second: %v", err)
	}

	firstHash, err := factorysessionexecution.IdempotencyTupleHash(first)
	if err != nil {
		t.Fatalf("hash first: %v", err)
	}
	secondHash, err := factorysessionexecution.IdempotencyTupleHash(second)
	if err != nil {
		t.Fatalf("hash second: %v", err)
	}
	if firstHash == secondHash {
		t.Fatalf("hash should differ when args change")
	}
}

func TestCheckRequestIDReplay_ConflictsOnDifferentTuple(t *testing.T) {
	err := factorysessionexecution.CheckRequestIDReplay("req-1", "sha256:abc", "sha256:def")
	if !errors.Is(err, factorysessionexecution.ErrExecutionRequestIDConflict) {
		t.Fatalf("error = %v, want ErrExecutionRequestIDConflict", err)
	}
}

func TestCheckRequestIDReplay_AllowsReplay(t *testing.T) {
	if err := factorysessionexecution.CheckRequestIDReplay("req-1", "sha256:abc", "sha256:abc"); err != nil {
		t.Fatalf("error = %v, want nil for replay", err)
	}
}

func TestInspectionLinksForSession(t *testing.T) {
	links := factorysessionexecution.InspectionLinksForSession("dur-sess-001", true)
	if links.Session != "/factory-sessions/dur-sess-001" {
		t.Fatalf("session link = %q", links.Session)
	}
	if links.Events != "/factory-sessions/dur-sess-001/events" {
		t.Fatalf("events link = %q", links.Events)
	}
}
