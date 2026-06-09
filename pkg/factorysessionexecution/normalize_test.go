package factorysessionexecution_test

import (
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

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
