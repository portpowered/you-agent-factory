package support_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCountWorkAtCustomerState_CountsListedWorkByWorkTypeAndState(t *testing.T) {
	workType := "task"
	complete := "complete"
	failed := "failed"
	listed := factoryapi.ListWorkResponse{
		Results: []factoryapi.Work{
			{
				WorkId:       strPtr("work-1"),
				WorkTypeName: &workType,
				State:        &factoryapi.WorkState{Name: complete, Type: factoryapi.WorkStateTypeTERMINAL},
			},
			{
				WorkId:       strPtr("work-2"),
				WorkTypeName: &workType,
				State:        &factoryapi.WorkState{Name: complete, Type: factoryapi.WorkStateTypeTERMINAL},
			},
			{
				WorkId:       strPtr("work-3"),
				WorkTypeName: &workType,
				State:        &factoryapi.WorkState{Name: failed, Type: factoryapi.WorkStateTypeFAILED},
			},
		},
	}

	if got := support.CountWorkAtCustomerState(listed, "task:complete"); got != 2 {
		t.Fatalf("CountWorkAtCustomerState(task:complete) = %d, want 2", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("CountWorkAtCustomerState(task:failed) = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:init"); got != 0 {
		t.Fatalf("CountWorkAtCustomerState(task:init) = %d, want 0", got)
	}
}

func TestHasWorkAtCustomerState_MatchesSpecificWorkID(t *testing.T) {
	workType := "goal"
	initState := "init"
	complete := "complete"
	listed := factoryapi.ListWorkResponse{
		Results: []factoryapi.Work{
			{
				WorkId:       strPtr("goal-1"),
				WorkTypeName: &workType,
				State:        &factoryapi.WorkState{Name: initState, Type: factoryapi.WorkStateTypeINITIAL},
			},
			{
				WorkId:       strPtr("goal-2"),
				WorkTypeName: &workType,
				State:        &factoryapi.WorkState{Name: complete, Type: factoryapi.WorkStateTypeTERMINAL},
			},
		},
	}

	if !support.HasWorkAtCustomerState(listed, "goal-2", "goal:complete") {
		t.Fatal("HasWorkAtCustomerState(goal-2, goal:complete) = false, want true")
	}
	if support.HasWorkAtCustomerState(listed, "goal-1", "goal:complete") {
		t.Fatal("HasWorkAtCustomerState(goal-1, goal:complete) = true, want false")
	}
	if support.HasWorkAtCustomerState(listed, "goal-2", "goal:init") {
		t.Fatal("HasWorkAtCustomerState(goal-2, goal:init) = true, want false")
	}
}

func TestWorkCustomerLocation_JoinsWorkTypeAndState(t *testing.T) {
	if got := support.WorkCustomerLocation("task", "complete"); got != "task:complete" {
		t.Fatalf("WorkCustomerLocation = %q, want %q", got, "task:complete")
	}
}

func TestWorkItemCustomerLocation_ReadsPublicWorkFields(t *testing.T) {
	workType := "scheduled"
	state := "complete"
	item := factoryapi.Work{
		WorkTypeName: &workType,
		State:        &factoryapi.WorkState{Name: state, Type: factoryapi.WorkStateTypeTERMINAL},
	}
	if got := support.WorkItemCustomerLocation(item); got != "scheduled:complete" {
		t.Fatalf("WorkItemCustomerLocation = %q, want %q", got, "scheduled:complete")
	}
	if got := support.WorkItemCustomerLocation(factoryapi.Work{}); got != "" {
		t.Fatalf("WorkItemCustomerLocation(empty) = %q, want empty", got)
	}
}

func TestCountWorkAtCustomerState_SupportBackedScenarioReachesTaskCompleteWithoutPetriHelpers(t *testing.T) {
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "public-work-location",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name": "worker",
		}},
		"workstations": []map[string]any{{
			"name":     "process",
			"worker":   "worker",
			"behavior": "STANDARD",
			"inputs":   []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"public observation"}`))

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     dir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, server.URL())
	support.WaitForSessionWorkTerminalFromFactoryEvents(t, server.URL(), "~default", 10*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "complete")); got != 1 {
		t.Fatalf("task:complete work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("task:init work count = %d, want 0; listed=%#v", got, listed)
	}
	workID := support.StringPointerValue(listed.Results[0].WorkId)
	if workID == "" || !support.HasWorkAtCustomerState(listed, workID, "task:complete") {
		t.Fatalf("HasWorkAtCustomerState(%q, task:complete) failed; listed=%#v", workID, listed)
	}
	server.Stop(t)
	terminalObservation.Wait(10 * time.Second)
}

func strPtr(value string) *string {
	return &value
}
