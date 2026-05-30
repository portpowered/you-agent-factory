package api

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSubmitWorkResponseFromResult_IdempotentReplayPreservesWorkIdentity(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	request := interfaces.WorkRequest{
		RequestID: "request-idem-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "draft-prd",
			WorkTypeID: "prd",
			TraceID:    "trace-idem-1",
		}},
	}

	first, err := mf.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := mf.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate submit: %v", err)
	}

	resp1 := submitWorkResponseFromResult(first, "")
	resp2 := submitWorkResponseFromResult(second, "")
	if !resp1.Accepted || resp2.Accepted {
		t.Fatalf("accepted flags = %v/%v, want true then false", resp1.Accepted, resp2.Accepted)
	}
	if resp1.RequestId != "request-idem-1" || resp2.RequestId != resp1.RequestId {
		t.Fatalf("requestId = %q/%q, want request-idem-1", resp1.RequestId, resp2.RequestId)
	}
	if resp1.TraceId != "trace-idem-1" || resp2.TraceId != resp1.TraceId {
		t.Fatalf("traceId = %q/%q, want stable trace-idem-1", resp1.TraceId, resp2.TraceId)
	}
	if stringValue(resp1.WorkId) != "batch-request-idem-1-draft-prd" || stringValue(resp2.WorkId) != stringValue(resp1.WorkId) {
		t.Fatalf("workId = %q/%q, want batch-request-idem-1-draft-prd", stringValue(resp1.WorkId), stringValue(resp2.WorkId))
	}
	if stringValue(resp1.Name) != "draft-prd" || stringValue(resp2.Name) != "draft-prd" {
		t.Fatalf("name = %q/%q, want draft-prd", stringValue(resp1.Name), stringValue(resp2.Name))
	}
	if stringValue(resp1.WorkTypeName) != "prd" || stringValue(resp2.WorkTypeName) != "prd" {
		t.Fatalf("workTypeName = %q/%q, want prd", stringValue(resp1.WorkTypeName), stringValue(resp2.WorkTypeName))
	}
}
