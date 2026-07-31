package work_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

func TestMaterializeWorkerOutputRootAssignsCanonicalIDs(t *testing.T) {
	t.Parallel()

	ids := 0
	service := workwire.NewRuntimeService(nil, nil, nil, nil)
	result, err := service.MaterializeWorkerOutput(context.Background(), work.MaterializeWorkerOutputRequest{
		Lineage: work.MaterializationLineageContext{
			DispatchID:             "dispatch-1",
			ParentWorkID:           "work-source",
			SourceWorkIDs:          []string{"work-source"},
			TraceID:                "trace-1",
			CurrentChainingTraceID: "chain-1",
		},
		Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "worker output",
		}},
		ProposedWork: []work.ProposedWorkItem{{
			WorkTypeID: "review",
			Name:       "review-out",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "body",
			}},
		}},
		ValidWorkTypes: map[string]bool{"review": true},
		IDGenerator: func() string {
			ids++
			return "gen"
		},
	})
	if err != nil {
		t.Fatalf("MaterializeWorkerOutput() error = %v", err)
	}
	if result.PrimaryOutput != "worker output" {
		t.Fatalf("PrimaryOutput = %q", result.PrimaryOutput)
	}
	if len(result.MaterializedWork) != 1 || result.MaterializedWork[0].ID != "work-gen" {
		t.Fatalf("MaterializedWork = %#v", result.MaterializedWork)
	}
	if result.MaterializedWork[0].ParentID != "work-source" {
		t.Fatalf("ParentID = %q", result.MaterializedWork[0].ParentID)
	}
}

func TestMaterializeWorkerOutputRootRejectsUnknownType(t *testing.T) {
	t.Parallel()

	service := workwire.NewRuntimeService(nil, nil, nil, nil)
	_, err := service.MaterializeWorkerOutput(context.Background(), work.MaterializeWorkerOutputRequest{
		ProposedWork: []work.ProposedWorkItem{{
			WorkTypeID: "missing",
			Name:       "x",
		}},
		ValidWorkTypes: map[string]bool{"task": true},
		IDGenerator:    func() string { return "1" },
	})
	if err == nil || !errors.Is(err, work.ErrUnknownProposedWorkType) {
		t.Fatalf("error = %v, want ErrUnknownProposedWorkType", err)
	}
}

func TestMaterializeWorkerOutputRootHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := workwire.NewRuntimeService(nil, nil, nil, nil)
	_, err := service.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestProposedWorkItemCloneDetachesCollections(t *testing.T) {
	t.Parallel()

	original := work.ProposedWorkItem{
		WorkTypeID: "task",
		Name:       "a",
		Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "x"}},
		Tags:       map[string]string{"k": "v"},
		Relations:  []work.Relation{{Type: work.RelationDependsOn, TargetWorkID: "other"}},
	}
	clone := original.Clone()
	original.Content[0].Text = "mutated"
	original.Tags["k"] = "mutated"
	original.Relations[0].TargetWorkID = "mutated"
	if clone.Content[0].Text != "x" || clone.Tags["k"] != "v" || clone.Relations[0].TargetWorkID != "other" {
		t.Fatalf("clone mutated with original: %#v", clone)
	}
}
