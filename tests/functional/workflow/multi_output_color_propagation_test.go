package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
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
