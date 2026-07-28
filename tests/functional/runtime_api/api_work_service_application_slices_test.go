package runtime_api

import (
	"context"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

// TestWorkServiceApplicationSlicesExerciseFunctionalLane proves published Work
// application-service methods used by Factory Sessions admission and invocation
// edges execute in the functional coverage lane.
func TestWorkServiceApplicationSlicesExerciseFunctionalLane(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := &functionalLaneWorkRuntime{}
	service := workwire.NewRuntimeService(
		functionalLaneRuntimeResolver{runtime: runtime},
		os.ReadFile,
		&functionalLaneContentStaging{},
		work.ContentMaterializeFunc(func(context.Context, string) (string, work.ContentCleanup, error) {
			return "/tmp/functional-materialized", func() {}, nil
		}),
	)

	preparedRequest, err := service.PrepareWorkRequest(ctx, work.WorkRequestPreparation{
		Request: work.WorkRequest{
			RequestID: "functional-prep",
			Works: []work.Work{{
				WorkTypeID: "task",
				Name:       "functional admission prep",
			}},
		},
		DefaultWorkTypeID: "task",
	})
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if preparedRequest.RequestID != "functional-prep" {
		t.Fatalf("prepared request = %#v, want request id functional-prep", preparedRequest)
	}

	preparedInput, err := service.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Signature: &work.InvocationSignatureConfig{
			Parameters: []work.InvocationParameterConfig{{
				Name:     "input",
				Bindings: []work.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}},
			}},
		},
		DirectArgs: []work.NamedArgumentInput{{Key: "input", Values: []string{"lane-draft"}}},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if preparedInput.NormalizedArguments == nil ||
		preparedInput.NormalizedArguments.Arguments["input"].Values[0] != "lane-draft" {
		t.Fatalf("prepared input = %#v, want direct args normalization", preparedInput)
	}

	submitted, err := service.SubmitWorkRequestForSession(ctx, "session-functional", preparedRequest)
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if submitted.RequestID != "functional-prep" {
		t.Fatalf("submit result = %#v, want request id functional-prep", submitted)
	}

	listed, err := service.ListWork(ctx, "session-functional", work.ListOptions{WorkTypeName: "task"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(listed.Results) != 1 || listed.Results[0].WorkID != "work-functional-1" {
		t.Fatalf("listed work = %#v, want one submitted work item", listed.Results)
	}

	selection, err := service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
		RequestID: "functional-prep",
		WorldState: work.InvocationWorldState{
			WorkRequestsByID: map[string]work.InvocationWorkRequest{
				"functional-prep": {WorkItems: []work.FactoryWorkItem{{
					ID: "work-functional-1", WorkTypeID: "task", State: "complete",
					Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "lane terminal"}},
				}}},
			},
			TerminalWorkByID: map[string]work.InvocationTerminalWork{
				"work-functional-1": {
					WorkItem: work.FactoryWorkItem{
						ID: "work-functional-1", State: "complete",
						Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "lane terminal"}},
					},
					Status: "TERMINAL",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("ResolvePrimaryResult: %v", err)
	}
	if len(selection.PrimaryResult) != 1 || selection.PrimaryResult[0].Text != "lane terminal" {
		t.Fatalf("selection = %#v, want terminal primary output", selection)
	}

	staged, err := service.StageContent(ctx, work.StageContentRequest{
		FileName:  "functional.png",
		MediaType: "image/png",
		Content:   []byte("png"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" {
		t.Fatalf("stage result = %#v, want staged file ref", staged)
	}

	materialized, cleanup, err := service.MaterializeContentURL(ctx, "file:///functional.png")
	if err != nil {
		t.Fatalf("MaterializeContentURL: %v", err)
	}
	if materialized != "/tmp/functional-materialized" {
		t.Fatalf("materialized path = %q, want /tmp/functional-materialized", materialized)
	}
	if cleanup != nil {
		cleanup()
	}
}

type functionalLaneRuntimeResolver struct {
	runtime work.Runtime
}

func (r functionalLaneRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, nil
}

type functionalLaneWorkRuntime struct {
	submitted work.WorkRequest
}

func (r *functionalLaneWorkRuntime) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	r.submitted = request
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		TraceID:   "trace-functional",
		WorkID:    "work-functional-1",
		Works: []work.WorkRequestSubmittedWork{{
			WorkID:       "work-functional-1",
			WorkTypeName: "task",
			Name:         "functional admission prep",
		}},
	}, nil
}

func (r *functionalLaneWorkRuntime) MoveWork(
	_ context.Context,
	_ string,
	_ string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	return work.OperatorMoveResult{}, nil
}

func (r *functionalLaneWorkRuntime) ReadWorkSnapshot(_ context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{
		Items: []work.ReadModel{{
			WorkID:       "work-functional-1",
			WorkTypeName: "task",
			Name:         "functional admission prep",
		}},
	}, nil
}

type functionalLaneContentStaging struct{}

func (functionalLaneContentStaging) StageContent(
	_ context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	return work.StageContentResult{
		StagedFileRef: "functional-stage-ref",
		FileName:      request.FileName,
		MediaType:     request.MediaType,
	}, nil
}

func (functionalLaneContentStaging) PrepareContent(context.Context, []work.StagedSubmissionItem) ([]work.WorkContentPart, error) {
	return nil, nil
}

func (functionalLaneContentStaging) ResolveContent(context.Context, string) (work.ResolvedStagedContent, error) {
	return work.ResolvedStagedContent{}, nil
}

func (functionalLaneContentStaging) CleanupContent(context.Context, string) error {
	return nil
}
