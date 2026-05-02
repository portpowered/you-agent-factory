package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestMultiOutputColorPropagation(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output cross-type color propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

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

	h.Assert().
		HasTokenInPlace("idea:complete").
		HasTokenInPlace("task:complete").
		HasNoTokenInPlace("idea:init").
		HasNoTokenInPlace("task:init")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.PlaceID == "task:complete" {
			if tok.Color.Name != "my-feature-plan" {
				t.Errorf("task:complete Name: want 'my-feature-plan, got %q", tok.Color.Name)
			}
			if tok.Color.WorkID == "work-idea-001" {
				t.Error("task:complete WorkID should be fresh, got input's WorkID")
			}
			if tok.Color.WorkID == "" {
				t.Error("task:complete WorkID should not be empty")
			}
			if tok.Color.TraceID != "trace-multi-out" {
				t.Errorf("task:complete TraceID: want 'trace-multi-out', got %q", tok.Color.TraceID)
			}
			if len(tok.Color.Tags) > 0 {
				t.Errorf("task:complete Tags should be empty for cross-type, got %v", tok.Color.Tags)
			}
			if tok.Color.WorkTypeID != "task" {
				t.Errorf("task:complete WorkTypeID: want 'task', got %q", tok.Color.WorkTypeID)
			}
			if tok.Color.ParentID != "work-idea-001" {
				t.Errorf("task:complete ParentID: want 'work-idea-001', got %q", tok.Color.ParentID)
			}
			return
		}
	}
	t.Error("no token found in task:complete")
}

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

func TestMultiOutputColorPropagation_NameAvailableDownstream(t *testing.T) {
	support.SkipLongFunctional(t, "slow multi-output downstream-name propagation sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "prd-factory-log-levels",
		WorkID:     "work-idea-002",
		WorkTypeID: "idea",
		TraceID:    "trace-name-downstream",
		Payload:    []byte("idea about logging"),
	})

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithCommandRunner(successRunner("downstream-ok")),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 5*time.Second)

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
