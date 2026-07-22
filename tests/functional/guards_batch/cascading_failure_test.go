package guards_batch

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCascadingFailure_DirectChild(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	parentWorkID := "parent-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID:  "task",
		WorkID:      parentWorkID,
		TargetState: "processing",
		Payload:     []byte("parent"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("child"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: parentWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"finisher": {
			{Error: errors.New("upstream service down")},
		},
	})
	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:failed": 2, "task:init": 0, "task:processing": 0, "task:complete": 0})
	assertFailedDependentWork(t, listedWork, parentWorkID)
}

func TestCascadingFailure_Transitive(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	pWorkID := "P-work-id"
	c1WorkID := "C1-work-id"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     pWorkID,
		Payload:    []byte("P"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     c1WorkID,
		Payload:    []byte("C1"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: pWorkID, RequiredState: "complete"},
		},
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("C2"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: c1WorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Error: errors.New("crash")},
			{Error: errors.New("crash")},
			{Error: errors.New("crash")},
		},
	})
	session, listedWork := support.RunFactoryToCompletionWithEdgesAndWork(t, dir, serviceedges.Edges{
		ProviderOverride: provider,
	}, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:failed": 3, "task:init": 0, "task:processing": 0, "task:complete": 0})
	assertFailedDependentWork(t, listedWork, pWorkID)
	assertFailedDependentWork(t, listedWork, c1WorkID)
}

func TestCascadingFailure_CompletedNotCascaded(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	aWorkID := "A-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     aWorkID,
		Payload:    []byte("A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: aWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Content: "COMPLETE"},
			{Error: errors.New("oops")},
		},
	})
	session := support.RunFactoryToCompletion(t, dir, provider, 10*time.Second)
	assertGuardSessionPlaces(t, session, map[string]int{"task:complete": 1, "task:failed": 1})
}

func assertFailedDependentWork(t *testing.T, response factoryapi.ListWorkResponse, targetWorkID string) {
	t.Helper()
	for _, item := range response.Results {
		if item.State == nil || item.State.Name != "failed" || item.Relations == nil {
			continue
		}
		for _, relation := range *item.Relations {
			if relation.Type == factoryapi.RelationTypeDependsOn && relation.TargetWorkId != nil && *relation.TargetWorkId == targetWorkID {
				return
			}
		}
	}
	t.Errorf("listed failed Work missing dependency on %q", targetWorkID)
}
