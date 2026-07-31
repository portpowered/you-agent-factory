package workers_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProposedOutputFromLegacyWorkResultDropsCanonicalIDs(t *testing.T) {
	t.Parallel()

	output := workers.ProposedOutputFromLegacyWorkResult(workers.WorkResult{
		Output:   "accepted body",
		Feedback: "ok",
		RecordedOutputWork: []work.FactoryWorkItem{{
			ID:          "work-agent-chosen",
			WorkTypeID:  "review",
			DisplayName: "review-1",
			State:       "init",
			PlaceID:     "review:init",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "review body",
			}},
			Tags: map[string]string{"id": "work-agent-chosen", "source": "envelope"},
		}},
	})
	if len(output.Primary) != 1 || output.Primary[0].Text != "accepted body" {
		t.Fatalf("Primary = %#v", output.Primary)
	}
	if output.Feedback != "ok" {
		t.Fatalf("Feedback = %q", output.Feedback)
	}
	if len(output.ProposedWork) != 1 {
		t.Fatalf("ProposedWork = %#v", output.ProposedWork)
	}
	item := output.ProposedWork[0]
	if item.Name != "review-1" || item.WorkTypeID != "review" {
		t.Fatalf("proposal = %#v", item)
	}
	if _, ok := item.Tags["id"]; ok {
		t.Fatalf("identity tag leaked: %#v", item.Tags)
	}
	if item.Tags["source"] != "envelope" {
		t.Fatalf("tags = %#v", item.Tags)
	}
}

func TestWorkProposedItemsFromProposedWorkClones(t *testing.T) {
	t.Parallel()

	source := []workers.ProposedWork{{
		WorkTypeID: "task",
		Name:       "a",
		Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "body"}},
		Tags:       map[string]string{"k": "v"},
	}}
	got := workers.WorkProposedItemsFromProposedWork(source)
	source[0].Content[0].Text = "mutated"
	source[0].Tags["k"] = "mutated"
	if got[0].Content[0].Text != "body" || got[0].Tags["k"] != "v" {
		t.Fatalf("clone leaked mutation: %#v", got[0])
	}
}
