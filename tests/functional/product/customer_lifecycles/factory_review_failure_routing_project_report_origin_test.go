package customer_lifecycles_test

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestProjectReport_CarriesTheExactFailedChildWorkID proves a project-report
// minted for a different work type records the failed child's own Work ID as
// its parent, so a lead can recover the origin without matching on names. A
// same-name restoration under a different request must point at its own ID.
func testFactoryreviewfailureroutingProjectReport_CarriesTheExactFailedChildWorkID(t *testing.T) {
	t.Parallel()
	wake := openProjectWakeScenario(t)
	wake.admitOwner(t)
	stream := wake.eventStream(t)

	slug := "origin-slice"
	name := wake.ownerName + "-" + slug
	submitChild := func(suffix string) string {
		ideaID := wake.marker + "-" + slug + "-" + suffix + "-idea"
		taskID := wake.marker + "-" + slug + "-" + suffix + "-task"
		traceID := wake.marker + "-" + slug + "-" + suffix + "-trace"
		wake.submit(t, taskID+"-request", reviewFailureSeed{
			Name: name, WorkID: taskID, WorkType: "task", State: "failed",
			TraceID: traceID, Payload: slug + " task",
		})
		wake.submit(t, ideaID+"-request", reviewFailureSeed{
			Name: name, WorkID: ideaID, WorkType: "idea", State: "to-complete",
			TraceID: traceID, Payload: slug + " idea", Tags: map[string]string{"project": wake.ownerName},
		})
		return ideaID
	}

	firstID := submitChild("first")
	firstReports := assertProjectLeadWakes(t, wake, stream, name)
	secondID := submitChild("second")
	awaitReviewFailureDispatchResponses(t, stream, projectLeadWakeTransition, 1)

	works := wake.listWorks(t)
	reports := map[string]factoryapi.Work{}
	for _, work := range works {
		if work.State != nil && work.WorkTypeName != nil && *work.WorkTypeName == "project-report" {
			reports[*work.WorkId] = work
		}
	}
	if len(reports) != 2 {
		t.Fatalf("project-report works = %d, want 2: %#v", len(reports), reports)
	}
	origins := map[string]bool{}
	for id, report := range reports {
		origin := reportOriginWorkID(report)
		if origin == "" {
			t.Fatalf("project-report %q has no parent origin Work ID: %#v", id, report)
		}
		origins[origin] = true
	}
	if !origins[firstID] || !origins[secondID] || len(origins) != 2 {
		t.Fatalf("project-report origins = %v, want exactly %q and %q", origins, firstID, secondID)
	}
	if got := reportOriginWorkID(reports[firstReports[0]]); got != firstID {
		t.Fatalf("first report origin = %q, want %q", got, firstID)
	}
}

func reportOriginWorkID(work factoryapi.Work) string {
	if work.Relations == nil {
		return ""
	}
	for _, relation := range *work.Relations {
		if relation.Type == factoryapi.RelationTypeParentChild && relation.TargetWorkId != nil {
			return *relation.TargetWorkId
		}
	}
	return ""
}
