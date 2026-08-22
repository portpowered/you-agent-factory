package workers_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestProposedWorkCloneDetachesMutableFields(t *testing.T) {
	t.Parallel()

	source := workers.ProposedWork{
		WorkTypeID: "task",
		Name:       "a",
		State:      "init",
		Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "body"}},
		Tags:       map[string]string{"k": "v"},
		Relations:  []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "other"}},
	}

	clone := source.Clone()

	source.Content[0].Text = "mutated"
	source.Tags["k"] = "mutated"
	source.Relations[0].TargetWorkID = "mutated"

	if clone.Content[0].Text != "body" {
		t.Fatalf("Content mutation leaked into clone: %#v", clone.Content)
	}
	if clone.Tags["k"] != "v" {
		t.Fatalf("Tags mutation leaked into clone: %#v", clone.Tags)
	}
	if clone.Relations[0].TargetWorkID != "other" {
		t.Fatalf("Relations mutation leaked into clone: %#v", clone.Relations)
	}
	if clone.WorkTypeID != "task" || clone.Name != "a" || clone.State != "init" {
		t.Fatalf("scalar fields not preserved: %#v", clone)
	}
}

func TestProposedWorkCloneHandlesEmptyFields(t *testing.T) {
	t.Parallel()

	clone := workers.ProposedWork{WorkTypeID: "task"}.Clone()
	if clone.Content != nil || clone.Tags != nil || clone.Relations != nil {
		t.Fatalf("expected nil slices/maps to stay nil, got %#v", clone)
	}
}

func TestProposedOutputCloneDetachesNestedProposalsAndArtifacts(t *testing.T) {
	t.Parallel()

	source := workers.ProposedOutput{
		Primary:        []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "primary"}},
		Feedback:       "ok",
		Classification: "accepted",
		ProposedWork: []workers.ProposedWork{{
			WorkTypeID: "task",
			Name:       "a",
			Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "body"}},
			Tags:       map[string]string{"k": "v"},
		}},
		ArtifactRefs: []workers.ArtifactRef{{ArtifactID: "art-1", Label: "l", URI: "u"}},
	}

	clone := source.Clone()

	source.Primary[0].Text = "mutated"
	source.ProposedWork[0].Content[0].Text = "mutated"
	source.ProposedWork[0].Tags["k"] = "mutated"
	source.ArtifactRefs[0].ArtifactID = "mutated"

	if clone.Primary[0].Text != "primary" {
		t.Fatalf("Primary mutation leaked into clone: %#v", clone.Primary)
	}
	if clone.ProposedWork[0].Content[0].Text != "body" {
		t.Fatalf("nested ProposedWork Content mutation leaked into clone: %#v", clone.ProposedWork)
	}
	if clone.ProposedWork[0].Tags["k"] != "v" {
		t.Fatalf("nested ProposedWork Tags mutation leaked into clone: %#v", clone.ProposedWork)
	}
	if clone.ArtifactRefs[0].ArtifactID != "art-1" {
		t.Fatalf("ArtifactRefs mutation leaked into clone: %#v", clone.ArtifactRefs)
	}
	if clone.Feedback != "ok" || clone.Classification != "accepted" {
		t.Fatalf("scalar fields not preserved: %#v", clone)
	}
}

func TestProposedOutputCloneHandlesEmptyFields(t *testing.T) {
	t.Parallel()

	clone := workers.ProposedOutput{Feedback: "ok"}.Clone()
	if clone.Primary != nil || clone.ProposedWork != nil || clone.ArtifactRefs != nil {
		t.Fatalf("expected nil slices to stay nil, got %#v", clone)
	}
	if clone.Feedback != "ok" {
		t.Fatalf("Feedback not preserved: %#v", clone)
	}
}
