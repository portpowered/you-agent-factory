package relationships

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestMultiOutputFanoutPreservesSourceNameOnDownstreamWork proves that when one
// workstation fans out to multiple downstream work types, every completed branch
// retains the submitted source name and trace identity while receiving distinct
// generated Work IDs.
func testMultiOutputFanoutPreservesSourceNameOnDownstreamWork(t *testing.T, host *sharedRelationshipHost) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "my-feature-plan",
		WorkID:     "work-idea-001",
		WorkTypeID: "idea",
		TraceID:    "trace-multi-out",
		Payload:    []byte("idea payload"),
		Tags:       map[string]string{"priority": "high"},
	})

	_, listed, _ := runSharedRelationshipFactoryToCompletion(t, host, dir, 5*time.Second)

	assertRelationshipWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "complete"): 1,
		support.WorkCustomerLocation("task", "complete"): 1,
		support.WorkCustomerLocation("idea", "init"):     0,
		support.WorkCustomerLocation("task", "init"):     0,
	})
	assertDownstreamWorkPreservesSourceIdentity(
		t,
		listed,
		"task",
		"my-feature-plan",
		"work-idea-001",
		"trace-multi-out",
		map[string]string{"priority": "high"},
	)
}

// TestMultiOutputNameAvailableOnDownstreamTask proves that a downstream work type
// created by multi-output fanout can observe the submitted source name even when
// only the downstream branch reaches a terminal state in the public Work listing.
func testMultiOutputNameAvailableOnDownstreamTask(t *testing.T, host *sharedRelationshipHost) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_output_color_propagation"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "prd-factory-log-levels",
		WorkID:     "work-idea-002",
		WorkTypeID: "idea",
		TraceID:    "trace-name-downstream",
		Payload:    []byte("idea about logging"),
	})

	_, listed, _ := runSharedRelationshipFactoryToCompletion(t, host, dir, 5*time.Second)

	assertRelationshipWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "complete"): 1,
	})
	assertDownstreamWorkPreservesSourceIdentity(
		t,
		listed,
		"task",
		"prd-factory-log-levels",
		"work-idea-002",
		"trace-name-downstream",
		nil,
	)
}

// TestReviewerFanoutPreservesSharedNameDownstream proves that a scripted
// multi-output reviewer fanout preserves the source document name on every
// downstream review branch with distinct generated Work IDs.
func testReviewerFanoutPreservesSharedNameDownstream(t *testing.T, host *sharedRelationshipHost) {
	dir := scaffoldRelationshipReviewerFanoutFactory(t)

	_, listed, _ := runSharedRelationshipFactoryToCompletion(t, host, dir, 5*time.Second)

	assertRelationshipWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("document", "complete"):     1,
		support.WorkCustomerLocation("review-alpha", "complete"): 1,
		support.WorkCustomerLocation("review-beta", "complete"):  1,
	})
	assertFanoutBranchesPreserveSharedName(
		t,
		listed,
		[]string{"review-alpha", "review-beta"},
		"source-doc-alpha",
		"work-document-1",
	)
}

// TestDocReviewerPNGFanoutPreservesSharedNameDownstream proves that a packaged
// doc-reviewer factory fans out to every authored review branch while preserving
// the submitted document name on each downstream Work item.
func testDocReviewerPNGFanoutPreservesSharedNameDownstream(t *testing.T, host *sharedRelationshipHost) {
	dir := support.ScaffoldFactoryFromExamplePNG(t, "examples/factories/doc-reviewer.png")

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "source-doc-from-png",
		WorkID:     "work-document-png-1",
		WorkTypeID: "document",
		TraceID:    "trace-doc-reviewer-png",
		Payload:    []byte("review this document"),
	})

	_, listed, _ := runSharedRelationshipFactoryToCompletion(t, host, dir, 5*time.Second)

	assertRelationshipWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("document", "complete"):                 1,
		support.WorkCustomerLocation("review-task-normal-human", "complete"): 1,
		support.WorkCustomerLocation("review-task-reviewer", "complete"):     1,
	})
	assertFanoutBranchesPreserveSharedName(
		t,
		listed,
		[]string{"review-task-normal-human", "review-task-reviewer"},
		"source-doc-from-png",
		"work-document-png-1",
	)
}

// TestNtoNTypeMatchingCompletesEveryAuthoredBranch proves that independent
// work types submitted into an N-to-N matching factory each complete at their
// authored terminal states with preserved names, Work IDs, trace IDs, and tags.
func testNtoNTypeMatchingCompletesEveryAuthoredBranch(t *testing.T, host *sharedRelationshipHost) {
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

	_, listed, _ := runSharedRelationshipFactoryToCompletion(t, host, dir, 5*time.Second)

	assertRelationshipWorkCustomerStates(t, listed, map[string]int{
		support.WorkCustomerLocation("idea", "complete"):   1,
		support.WorkCustomerLocation("design", "complete"): 1,
	})
	assertTerminalWorkIdentity(
		t,
		listed,
		"idea",
		"cool-idea",
		"work-idea-100",
		"trace-idea",
		map[string]string{"source": "brainstorm"},
	)
	assertTerminalWorkIdentity(
		t,
		listed,
		"design",
		"cool-design",
		"work-design-200",
		"trace-design",
		map[string]string{"source": "figma"},
	)
}

func scaffoldRelationshipReviewerFanoutFactory(t *testing.T) string {
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

func assertRelationshipWorkCustomerStates(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("%s work count = %d, want %d; listed=%#v", location, got, want, listed.Results)
		}
	}
}

func assertDownstreamWorkPreservesSourceIdentity(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType, wantName, sourceWorkID, traceID string,
	forbiddenSourceTags map[string]string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.Name != wantName {
			t.Fatalf("%s:complete name = %q, want %q", workType, item.Name, wantName)
		}
		if item.WorkId == nil || *item.WorkId == "" || *item.WorkId == sourceWorkID {
			t.Fatalf("%s:complete Work ID = %#v, want generated ID distinct from %q", workType, item.WorkId, sourceWorkID)
		}
		if item.TraceId == nil || *item.TraceId != traceID {
			t.Fatalf("%s:complete trace ID = %#v, want %q", workType, item.TraceId, traceID)
		}
		for key, want := range forbiddenSourceTags {
			if item.Tags != nil && (*item.Tags)[key] == want {
				t.Fatalf("%s:complete tag %s = %q, want source tag not propagated downstream", workType, key, want)
			}
		}
		return
	}
	t.Fatalf("listed Work missing %s:complete", workType)
}

func assertFanoutBranchesPreserveSharedName(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workTypes []string,
	wantName, sourceWorkID string,
) {
	t.Helper()
	for _, workType := range workTypes {
		found := false
		for _, item := range listed.Results {
			if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
				continue
			}
			found = true
			if item.Name != wantName {
				t.Fatalf("%s terminal name = %q, want %q", workType, item.Name, wantName)
			}
			if item.WorkId == nil || *item.WorkId == "" || *item.WorkId == sourceWorkID {
				t.Fatalf("%s terminal Work ID = %#v, want generated ID distinct from %q", workType, item.WorkId, sourceWorkID)
			}
		}
		if !found {
			t.Fatalf("listed Work missing %s:complete", workType)
		}
	}
}

func assertTerminalWorkIdentity(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType, name, workID, traceID string,
	tags map[string]string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType || item.State == nil || item.State.Name != "complete" {
			continue
		}
		if item.Name != name || item.WorkId == nil || *item.WorkId != workID || item.TraceId == nil || *item.TraceId != traceID {
			t.Fatalf("%s terminal Work identity = %#v, want name=%q workId=%q traceId=%q", workType, item, name, workID, traceID)
		}
		for key, want := range tags {
			if item.Tags == nil || (*item.Tags)[key] != want {
				t.Fatalf("%s terminal Work tag %s = %#v, want %q", workType, key, item.Tags, want)
			}
		}
		return
	}
	t.Fatalf("listed Work missing %s:complete", workType)
}
