package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMultiOutputColorPropagation verifies that when a workstation has more
// output arcs than the worker returns explicit output tokens, the transitioner
// propagates the input token's color (Name, WorkID, TraceID, Tags) to the
// extra output tokens instead of leaving them empty.
//
// Topology: idea:init → [split] → idea:complete + task:init → [finish-task] → task:complete
// The script worker at [split] returns 1 output token. The transitioner must
// propagate input color to the second output (task:init) so canonical name
// and other fields are available to downstream workstations.
func TestMultiOutputColorPropagation(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "my-feature-plan",
		WorkID:     "work-idea-001",
		WorkTypeID: "idea",
		TraceID:    "trace-multi-out",
		Payload:    []byte("idea payload"),
		Tags:       map[string]string{"priority": "high"},
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(successRunner("split-output")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	// Both outputs should have tokens.
	h.Assert().
		HasTokenInPlace("idea:complete").
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("task:init")

	// Verify cross-type propagation rules on the task:complete token.
	// The task output is a different work type than the input (idea→task),
	// so it follows "unmatched" rules: generated name, fresh WorkID, trace
	// propagated, tags NOT propagated.
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:complete" {
			// Name should be generated: {input_name}/{output_work_type}/{index}
			if tok.Color.Name != "my-feature-plan" {
				t.Errorf("task:complete Name: want 'my-feature-plan, got %q", tok.Color.Name)
			}
			// WorkID should be fresh (not the input's work-idea-001)
			if tok.Color.WorkID == "work-idea-001" {
				t.Error("task:complete WorkID should be fresh, got input's WorkID")
			}
			if tok.Color.WorkID == "" {
				t.Error("task:complete WorkID should not be empty")
			}
			// TraceID propagates for cross-type outputs.
			if tok.Color.TraceID != "trace-multi-out" {
				t.Errorf("task:complete TraceID: want 'trace-multi-out', got %q", tok.Color.TraceID)
			}
			// Tags do NOT propagate for cross-type outputs.
			if len(tok.Color.Tags) > 0 {
				t.Errorf("task:complete Tags should be empty for cross-type, got %v", tok.Color.Tags)
			}
			// WorkTypeID comes from the target place, not the input.
			if tok.Color.WorkTypeID != "task" {
				t.Errorf("task:complete WorkTypeID: want 'task', got %q", tok.Color.WorkTypeID)
			}
			// ParentID links back to the originating input's WorkID.
			if tok.Color.ParentID != "work-idea-001" {
				t.Errorf("task:complete ParentID: want 'work-idea-001', got %q", tok.Color.ParentID)
			}
			return
		}
	}
	t.Error("no token found in task:complete")
}

// TestNtoN_TypeMatching verifies that when a workstation has N inputs of
// different types and N outputs of those same types, each output inherits
// Name/TraceID/Tags from the type-matched input. WorkID is preserved for
// same-type transitions.
func TestNtoN_TypeMatching(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "n_to_n_type_matching"))

	// Seed both inputs.
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
		testutil.WithCommandRunner(successRunner("review-done")),
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
			// Same-type: WorkID is preserved
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
			// Same-type: WorkID is preserved
			if tok.Color.WorkID != "work-design-200" {
				t.Errorf("design:complete WorkID: want 'work-design-200' (preserved), got %q", tok.Color.WorkID)
			}
			if tok.Color.WorkTypeID != "design" {
				t.Errorf("design:complete WorkTypeID: want 'design', got %q", tok.Color.WorkTypeID)
			}
		}
	}
}

// TestMultiOutputColorPropagation_NameAvailableDownstream verifies that the
// propagated Name is available in downstream script executor arg templates.
// This is the exact scenario from the factory: plan workstation outputs to
// plan:init, setup-workspace reads the name from the token.
func TestMultiOutputColorPropagation_NameAvailableDownstream(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "prd-factory-log-levels",
		WorkID:     "work-idea-002",
		WorkTypeID: "idea",
		TraceID:    "trace-name-downstream",
		Payload:    []byte("idea about logging"),
	})

	// Use echoArgsRunner so we can see what args the finish-task worker received.
	// The finish-task AGENTS.md has args ["done"] which is plain — but the
	// important thing is the token at task:init has the Name propagated.
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(successRunner("downstream-ok")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

	// Verify the task token carries a generated name containing the original input name.
	// Cross-type outputs get name format: {input_name}/{output_work_type}/{index}
	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:complete" {
			if tok.Color.Name != "prd-factory-log-levels" {
				t.Errorf("downstream task Name: want 'prd-factory-log-levels', got %q", tok.Color.Name)
			}
			return
		}
	}
	t.Error("no token found in task:complete")
}
