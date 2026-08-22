package token

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkerTokenBoundaryPreservesWorkFactsAndAuthoredState(t *testing.T) {
	workerToken := workers.Token{
		ID:    "token-1",
		State: "ready",
		Color: workers.Color{
			Name:       "review request",
			WorkID:     "work-1",
			WorkTypeID: "review",
			DataType:   workers.DataTypeWork,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "preserve this content",
			}},
		},
	}

	runtimeToken := FromWorker(workerToken)
	if runtimeToken.PlaceID != "review:ready" {
		t.Fatalf("runtime place = %q, want qualified internal place", runtimeToken.PlaceID)
	}

	got := ToWorker(runtimeToken)
	if got.ID != workerToken.ID || got.State != workerToken.State || got.Color.WorkID != workerToken.Color.WorkID ||
		got.Color.WorkTypeID != workerToken.Color.WorkTypeID || len(got.Color.Content) != 1 ||
		got.Color.Content[0].Text != "preserve this content" {
		t.Fatalf("worker projection = %#v, want detached Work facts and state", got)
	}
}

func TestWorkerTokenBoundaryUsesLastQualifiedStateSegment(t *testing.T) {
	got := ToWorker(Token{PlaceID: "review:approval:ready"})
	if got.State != "ready" {
		t.Fatalf("state = %q, want final qualified segment", got.State)
	}
}
