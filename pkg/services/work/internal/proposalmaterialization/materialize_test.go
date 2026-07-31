package proposalmaterialization_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/proposalmaterialization"
	"github.com/portpowered/infinite-you/pkg/services/work/internal/requestadmission"
)

func TestMaterializeAssignsWorkOwnedIdentityAndLineage(t *testing.T) {
	t.Parallel()

	ids := []string{"req-1", "a", "b"}
	next := 0
	result, err := proposalmaterialization.Materialize(context.Background(), proposalmaterialization.Request{
		Lineage: proposalmaterialization.LineageContext{
			DispatchID:               "dispatch-1",
			SourceWorkIDs:            []string{"work-parent"},
			CurrentChainingTraceID:   "chain-1",
			PreviousChainingTraceIDs: []string{"chain-0"},
			ChainingTraceDepth:       2,
			TraceID:                  "trace-1",
		},
		Primary: []requestadmission.ContentPart{{
			Type: requestadmission.ContentPartTypeText,
			Text: "primary output",
		}},
		Feedback: "looks good",
		ProposedWork: []proposalmaterialization.ProposedWorkItem{
			{
				WorkTypeID: "review",
				Name:       "review-1",
				State:      "init",
				Content: []requestadmission.ContentPart{{
					Type: requestadmission.ContentPartTypeText,
					Text: "review body",
				}},
				Tags: map[string]string{"source": "worker"},
			},
			{
				WorkTypeID: "task",
				Name:       "follow-up",
				Content: []requestadmission.ContentPart{{
					Type: requestadmission.ContentPartTypeText,
					Text: "next step",
				}},
			},
		},
		ValidWorkTypes: map[string]bool{"review": true, "task": true},
		ValidStatesByType: map[string]map[string]bool{
			"review": {"init": true},
			"task":   {"init": true},
		},
		IDGenerator: func() string {
			id := ids[next]
			next++
			return id
		},
	})
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if result.PrimaryOutput != "primary output" || result.Feedback != "looks good" {
		t.Fatalf("primary/feedback = (%q,%q)", result.PrimaryOutput, result.Feedback)
	}
	if len(result.MaterializedWork) != 2 {
		t.Fatalf("materialized count = %d, want 2", len(result.MaterializedWork))
	}
	first := result.MaterializedWork[0]
	if first.ID != "work-a" {
		t.Fatalf("first ID = %q, want work-a (Work-owned)", first.ID)
	}
	if first.ParentID != "work-parent" {
		t.Fatalf("parent = %q, want work-parent", first.ParentID)
	}
	if first.CurrentChainingTraceID != "chain-1" || first.TraceID != "trace-1" {
		t.Fatalf("lineage = %#v", first)
	}
	if first.Tags["source"] != "worker" || first.Tags["_source_dispatch_id"] != "dispatch-1" {
		t.Fatalf("tags = %#v", first.Tags)
	}
	if result.MaterializedWork[1].ID != "work-b" {
		t.Fatalf("second ID = %q, want work-b", result.MaterializedWork[1].ID)
	}
}

func TestMaterializeRejectsUnknownWorkType(t *testing.T) {
	t.Parallel()

	_, err := proposalmaterialization.Materialize(context.Background(), proposalmaterialization.Request{
		ProposedWork: []proposalmaterialization.ProposedWorkItem{{
			WorkTypeID: "missing",
			Name:       "item",
		}},
		ValidWorkTypes: map[string]bool{"task": true},
		IDGenerator:    func() string { return "1" },
	})
	if err == nil || !errors.Is(err, proposalmaterialization.ErrUnknownWorkType) {
		t.Fatalf("error = %v, want ErrUnknownWorkType", err)
	}
}

func TestMaterializeRejectsUnknownState(t *testing.T) {
	t.Parallel()

	_, err := proposalmaterialization.Materialize(context.Background(), proposalmaterialization.Request{
		ProposedWork: []proposalmaterialization.ProposedWorkItem{{
			WorkTypeID: "task",
			Name:       "item",
			State:      "nope",
		}},
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true}},
		IDGenerator:       func() string { return "1" },
	})
	if err == nil || !errors.Is(err, proposalmaterialization.ErrInvalidProposal) {
		t.Fatalf("error = %v, want ErrInvalidProposal", err)
	}
	if !strings.Contains(err.Error(), "unknown state") {
		t.Fatalf("error = %v, want unknown state detail", err)
	}
}

func TestMaterializeIgnoresCallerSuppliedIdentityHints(t *testing.T) {
	t.Parallel()

	// Proposal tags may carry leftover identity metadata from legacy envelopes;
	// Work still assigns the canonical ID via the generator.
	result, err := proposalmaterialization.Materialize(context.Background(), proposalmaterialization.Request{
		Lineage: proposalmaterialization.LineageContext{RequestID: "request-fixed"},
		ProposedWork: []proposalmaterialization.ProposedWorkItem{{
			WorkTypeID: "task",
			Name:       "named",
			Tags:       map[string]string{"id": "agent-chosen-id"},
		}},
		ValidWorkTypes: map[string]bool{"task": true},
		IDGenerator:    func() string { return "owned-9" },
	})
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if got := result.MaterializedWork[0].ID; got != "work-owned-9" {
		t.Fatalf("ID = %q, want work-owned-9", got)
	}
}

func TestMaterializeEmptyProposalsReturnsPrimaryOnly(t *testing.T) {
	t.Parallel()

	result, err := proposalmaterialization.Materialize(context.Background(), proposalmaterialization.Request{
		Primary: []requestadmission.ContentPart{{
			Type: requestadmission.ContentPartTypeText,
			Text: "just text",
		}},
		Classification: "accepted",
	})
	if err != nil {
		t.Fatalf("Materialize() error = %v", err)
	}
	if result.PrimaryOutput != "just text" || result.Classification != "accepted" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.MaterializedWork) != 0 {
		t.Fatalf("materialized = %#v, want empty", result.MaterializedWork)
	}
}
