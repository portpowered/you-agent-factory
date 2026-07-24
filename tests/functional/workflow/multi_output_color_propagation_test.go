package workflow

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNtoN_TypeMatching(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "n_to_n_type_matching"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "cool-idea",
		WorkID:     "work-idea-100",
		WorkTypeID: "idea",
		TraceID:    "trace-idea",
		Payload:    []byte("idea content"),
		Tags:       map[string]string{"source": "brainstorm"},
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "cool-design",
		WorkID:     "work-design-200",
		WorkTypeID: "design",
		TraceID:    "trace-design",
		Payload:    []byte("design content"),
		Tags:       map[string]string{"source": "figma"},
	})

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("review-done"),
	}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{"idea:complete": 1, "design:complete": 1})
	assertTerminalWork(t, listedWork, "idea", "cool-idea", "work-idea-100", "trace-idea", map[string]string{"source": "brainstorm"})
	assertTerminalWork(t, listedWork, "design", "cool-design", "work-design-200", "trace-design", map[string]string{"source": "figma"})
}

func TestMultiOutputReviewerFanoutPreservesSharedNameDownstream(t *testing.T) {
	dir := scaffoldReviewerFanoutFactory(t)

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ScriptCommandRunner: support.NewStaticSuccessCommandRunner("done"),
	}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"document:complete": 1, "review-alpha:complete": 1, "review-beta:complete": 1,
	})
	assertFanoutTerminalNames(t, listedWork, []string{"review-alpha", "review-beta"}, "source-doc-alpha", "work-document-1")
}

func scaffoldReviewerFanoutFactory(t *testing.T) string {
	t.Helper()

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
		"workers": []map[string]any{{"name": "reviewer-worker"}},
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
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "source-doc-alpha",
		WorkID:     "work-document-1",
		WorkTypeID: "document",
		TraceID:    "trace-reviewer-fanout",
		Payload:    []byte("review this document"),
	})
	return dir
}

func TestDocReviewerExamplePNGFanoutPreservesSharedNameDownstream(t *testing.T) {
	dir := support.ScaffoldFactoryFromExamplePNG(t, "examples/factories/doc-reviewer.png")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "source-doc-from-png",
		WorkID:     "work-document-png-1",
		WorkTypeID: "document",
		TraceID:    "trace-doc-reviewer-png",
		Payload:    []byte("review this document"),
	})

	_, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("<COMPLETE>"),
	}, 5*time.Second)
	assertWorkflowSessionPlaces(t, listedWork, map[string]int{
		"document:complete":                 1,
		"review-task-normal-human:complete": 1,
		"review-task-reviewer:complete":     1,
	})
	assertFanoutTerminalNames(t, listedWork, []string{"review-task-normal-human", "review-task-reviewer"}, "source-doc-from-png", "work-document-png-1")
}

func assertTerminalWork(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workType, name, workID, traceID string,
	tags map[string]string,
) {
	t.Helper()
	for _, item := range response.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.Name != name || item.WorkId == nil || *item.WorkId != workID || item.TraceId == nil || *item.TraceId != traceID {
			t.Errorf("%s terminal Work identity = %#v", workType, item)
		}
		for key, want := range tags {
			if item.Tags == nil || (*item.Tags)[key] != want {
				t.Errorf("%s terminal Work tag %s = %#v, want %q", workType, key, item.Tags, want)
			}
		}
		return
	}
	t.Errorf("listed Work missing %s:complete", workType)
}

func assertFanoutTerminalNames(
	t *testing.T,
	response factoryapi.ListWorkResponse,
	workTypes []string,
	wantName, sourceWorkID string,
) {
	t.Helper()
	for _, workType := range workTypes {
		found := false
		for _, item := range response.Results {
			if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
				continue
			}
			found = true
			if item.Name != wantName {
				t.Errorf("%s terminal name = %q, want %q", workType, item.Name, wantName)
			}
			if item.WorkId == nil || *item.WorkId == "" || *item.WorkId == sourceWorkID {
				t.Errorf("%s terminal Work ID = %#v, want generated ID distinct from %q", workType, item.WorkId, sourceWorkID)
			}
		}
		if !found {
			t.Errorf("listed Work missing %s:complete", workType)
		}
	}
}
