package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNtoN_TypeMatching(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "n_to_n_type_matching"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "cool-idea",
		WorkID:     "work-idea-100",
		WorkTypeID: "idea",
		TraceID:    "trace-idea",
		Payload:    []byte("idea content"),
		Tags:       map[string]string{"source": "brainstorm"},
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "cool-design",
		WorkID:     "work-design-200",
		WorkTypeID: "design",
		TraceID:    "trace-design",
		Payload:    []byte("design content"),
		Tags:       map[string]string{"source": "figma"},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(support.NewStaticSuccessCommandRunner("review-done")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		HasTokenInPlace("idea:complete").
		HasTokenInPlace("design:complete")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		switch tok.PlaceID {
		case "idea:complete":
			if tok.Color.Name != "cool-idea" {
				t.Errorf("idea:complete Name: want 'cool-idea', got %q", tok.Color.Name)
			}
			if tok.Color.TraceID != "trace-idea" {
				t.Errorf("idea:complete TraceID: want 'trace-idea', got %q", tok.Color.TraceID)
			}
			if tok.Color.Tags["source"] != "brainstorm" {
				t.Errorf("idea:complete Tags[source]: want 'brainstorm', got %q", tok.Color.Tags["source"])
			}
			if tok.Color.WorkID != "work-idea-100" {
				t.Errorf("idea:complete WorkID: want 'work-idea-100' (preserved), got %q", tok.Color.WorkID)
			}
			if tok.Color.WorkTypeID != "idea" {
				t.Errorf("idea:complete WorkTypeID: want 'idea', got %q", tok.Color.WorkTypeID)
			}
		case "design:complete":
			if tok.Color.Name != "cool-design" {
				t.Errorf("design:complete Name: want 'cool-design', got %q", tok.Color.Name)
			}
			if tok.Color.TraceID != "trace-design" {
				t.Errorf("design:complete TraceID: want 'trace-design', got %q", tok.Color.TraceID)
			}
			if tok.Color.Tags["source"] != "figma" {
				t.Errorf("design:complete Tags[source]: want 'figma', got %q", tok.Color.Tags["source"])
			}
			if tok.Color.WorkID != "work-design-200" {
				t.Errorf("design:complete WorkID: want 'work-design-200' (preserved), got %q", tok.Color.WorkID)
			}
			if tok.Color.WorkTypeID != "design" {
				t.Errorf("design:complete WorkTypeID: want 'design', got %q", tok.Color.WorkTypeID)
			}
		}
	}
}

func TestMultiOutputReviewerFanoutPreservesSharedNameDownstream(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "reviewer_fanout_name_propagation",
		"workTypes": []map[string]any{
			{
				"name": "document",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "review-alpha",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "review-beta",
				"states": []map[string]any{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{"name": "reviewer-worker"},
		},
		"workstations": []map[string]any{
			{
				"name": "split-review",
				"inputs": []map[string]any{
					{"workType": "document", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "document", "state": "complete"},
					{"workType": "review-alpha", "state": "init"},
					{"workType": "review-beta", "state": "init"},
				},
				"onFailure": []map[string]any{
					{"workType": "document", "state": "failed"},
				},
				"worker": "reviewer-worker",
			},
			{
				"name": "review-alpha",
				"inputs": []map[string]any{
					{"workType": "review-alpha", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "review-alpha", "state": "complete"},
				},
				"onFailure": []map[string]any{
					{"workType": "review-alpha", "state": "failed"},
				},
				"worker": "reviewer-worker",
			},
			{
				"name": "review-beta",
				"inputs": []map[string]any{
					{"workType": "review-beta", "state": "init"},
				},
				"outputs": []map[string]any{
					{"workType": "review-beta", "state": "complete"},
				},
				"onFailure": []map[string]any{
					{"workType": "review-beta", "state": "failed"},
				},
				"worker": "reviewer-worker",
			},
		},
	})
	support.WriteAgentConfig(t, dir, "reviewer-worker", `---
args:
  - done
command: echo
type: SCRIPT_WORKER
---
`)

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "source-doc-alpha",
		WorkID:     "work-document-1",
		WorkTypeID: "document",
		TraceID:    "trace-reviewer-fanout",
		Payload:    []byte("review this document"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(support.NewStaticSuccessCommandRunner("done")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		HasTokenInPlace("document:complete").
		HasTokenInPlace("review-alpha:complete").
		HasTokenInPlace("review-beta:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}

	splitDispatches := dispatchesForWorkstation(snapshot.DispatchHistory, "split-review")
	if len(splitDispatches) != 1 {
		t.Fatalf("split-review dispatch count = %d, want 1", len(splitDispatches))
	}

	fanoutNames := map[string]string{}
	for _, mutation := range splitDispatches[0].OutputMutations {
		if mutation.Token == nil {
			continue
		}
		switch mutation.ToPlace {
		case "review-alpha:init", "review-beta:init":
			fanoutNames[mutation.ToPlace] = mutation.Token.Color.Name
			if mutation.Token.Color.Name != "source-doc-alpha" {
				t.Fatalf("%s generated name = %q, want source-doc-alpha", mutation.ToPlace, mutation.Token.Color.Name)
			}
			if mutation.Token.Color.WorkID == "" || mutation.Token.Color.WorkID == "work-document-1" {
				t.Fatalf("%s work ID = %q, want fresh generated reviewer work ID", mutation.ToPlace, mutation.Token.Color.WorkID)
			}
		}
	}
	if len(fanoutNames) != 2 {
		t.Fatalf("fanout output names = %#v, want both reviewer init lanes", fanoutNames)
	}

	for _, workstationName := range []string{"review-alpha", "review-beta"} {
		dispatches := dispatchesForWorkstation(snapshot.DispatchHistory, workstationName)
		if len(dispatches) != 1 {
			t.Fatalf("%s dispatch count = %d, want 1", workstationName, len(dispatches))
		}
		if len(dispatches[0].ConsumedTokens) != 1 {
			t.Fatalf("%s consumed token count = %d, want 1", workstationName, len(dispatches[0].ConsumedTokens))
		}
		if got := dispatches[0].ConsumedTokens[0].Color.Name; got != "source-doc-alpha" {
			t.Fatalf("%s consumed input name = %q, want source-doc-alpha", workstationName, got)
		}
	}

	marking := h.Marking()
	for _, placeID := range []string{"review-alpha:complete", "review-beta:complete"} {
		tokens := marking.TokensInPlace(placeID)
		if len(tokens) != 1 {
			t.Fatalf("%s token count = %d, want 1", placeID, len(tokens))
		}
		if tokens[0].Color.Name != "source-doc-alpha" {
			t.Fatalf("%s terminal name = %q, want source-doc-alpha", placeID, tokens[0].Color.Name)
		}
	}
}
