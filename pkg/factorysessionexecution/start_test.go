package factorysessionexecution_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
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

func TestNormalizeStartRequest_AcceptsFactoryIDSource(t *testing.T) {
	normalized, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-001",
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "customer-support-triage",
		},
		Args: map[string]any{"ticketId": "TKT-1001"},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if normalized.RequestID != "req-001" {
		t.Fatalf("requestId = %q, want req-001", normalized.RequestID)
	}
	if normalized.Source.FactoryID != "customer-support-triage" {
		t.Fatalf("factoryId = %q, want customer-support-triage", normalized.Source.FactoryID)
	}
}

func TestNormalizeStartRequest_RejectsMissingRequestID(t *testing.T) {
	_, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		Source: factorysessionexecution.Source{
			Kind:      workflowsource.KindFactoryID,
			FactoryID: "factory",
		},
	})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "requestId" {
		t.Fatalf("error = %v, want requestId validation error", err)
	}
}

func TestNormalizeStartRequest_RejectsMismatchedSourcePayload(t *testing.T) {
	_, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-002",
		Source: factorysessionexecution.Source{
			Kind: workflowsource.KindWorkflowFile,
		},
	})
	if err == nil {
		t.Fatal("error = nil, want workflowFile validation error")
	}
	var validationErr *factorysessionexecution.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "source.workflowFile" {
		t.Fatalf("error = %v, want source.workflowFile validation error", err)
	}
}

func TestNormalizeStartRequest_AcceptsInlineWorkflowSource(t *testing.T) {
	normalized, err := factorysessionexecution.NormalizeStartRequest(factorysessionexecution.StartRequest{
		RequestID: "req-inline",
		Source: factorysessionexecution.Source{
			Kind: workflowsource.KindInlineWorkflow,
			InlineWorkflow: &factorysessionexecution.InlineWorkflowSource{
				InlineSource: `meta({ name: "demo" });`,
				Dialect:      "you-workflow-v1",
			},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeStartRequest: %v", err)
	}
	if normalized.Source.InlineWorkflow.InlineSource == "" {
		t.Fatal("inline source is empty")
	}
}
