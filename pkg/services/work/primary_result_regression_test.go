package work_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

var primaryResultRegressionFixtures = []struct {
	name string
}{
	{name: "submitted_work_terminal"},
	{name: "explicit_policy"},
	{name: "accepted_response_not_submitted_input"},
}

func TestPrimaryResultRegression_FocusedFixturesRemainByteIdentical(t *testing.T) {
	t.Parallel()

	for _, tc := range primaryResultRegressionFixtures {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			selection, err := resolvePrimaryResultRegressionFixture(tc.name)
			if err != nil {
				t.Fatalf("resolvePrimaryResultRegressionFixture() error = %v", err)
			}

			got, err := json.Marshal(selection.PrimaryResult)
			if err != nil {
				t.Fatalf("json.Marshal(primaryResult) error = %v", err)
			}

			want := readPrimaryResultRegressionFixtureBytes(t, tc.name)
			if string(got) != string(want) {
				t.Fatalf("primary result bytes changed:\ngot=%s\nwant=%s", got, want)
			}
		})
	}
}

func resolvePrimaryResultRegressionFixture(name string) (work.PrimaryResultSelection, error) {
	switch name {
	case "submitted_work_terminal":
		state := primaryResultWorldStateFixture()
		rootInitial := primaryResultWorkItem("work-root", "task", "draft", "root", "task:init")
		rootTerminal := primaryResultWorkItem("work-root", "task", "complete", "root", "task:complete")
		recordPrimaryResultSubmittedWork(&state, 1, "request-1", rootInitial)
		recordPrimaryResultDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal)
		state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}
		return work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
			RequestID:  "request-1",
			WorldState: state,
		})

	case "explicit_policy":
		state := primaryResultWorldStateFixture()
		rootInitial := primaryResultWorkItem("work-root", "task", "draft", "root", "task:init")
		rootTerminal := primaryResultWorkItem("work-root", "task", "complete", "root", "task:complete")
		summaryTerminal := primaryResultWorkItem("work-summary", "summary", "complete", "summary", "summary:complete")
		recordPrimaryResultSubmittedWork(&state, 1, "request-1", rootInitial)
		recordPrimaryResultDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal, summaryTerminal)
		state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}
		state.TerminalWorkByID[summaryTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: summaryTerminal, Status: "TERMINAL"}
		return work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
			RequestID: "request-1",
			InvocationReturn: &interfaces.InvocationReturnConfig{
				Policy:        "EXPLICIT",
				WorkTypeName:  "summary",
				TerminalState: "complete",
				WorkName:      "summary",
			},
			WorldState: state,
		})

	case "accepted_response_not_submitted_input":
		state := primaryResultWorldStateFixture()
		requestContent := []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "submitted request text",
		}}
		responseContent := []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "accepted workstation response",
		}}
		rootInitial := primaryResultWorkItem("work-root", "task", "draft", "root", "task:init")
		rootInitial.Content = requestContent
		rootTerminal := primaryResultWorkItem("work-root", "task", "complete", "root", "task:complete")
		rootTerminal.Content = responseContent
		recordPrimaryResultSubmittedWork(&state, 1, "request-1", rootInitial)
		recordPrimaryResultDispatchOutput(&state, 2, "dispatch-root", []work.FactoryWorkItem{rootInitial}, rootTerminal)
		state.TerminalWorkByID[rootTerminal.ID] = interfaces.FactoryTerminalWork{WorkItem: rootTerminal, Status: "TERMINAL"}
		return work.ResolvePrimaryResult(work.PrimaryResultSelectionInput{
			RequestID:  "request-1",
			WorldState: state,
		})

	default:
		return work.PrimaryResultSelection{}, os.ErrNotExist
	}
}

func readPrimaryResultRegressionFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()

	fixturePath := filepath.Join("internal", "testdata", "primary_result_regression", name+".json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %q: %v", fixturePath, err)
	}
	return raw
}

func primaryResultWorldStateFixture() interfaces.FactoryWorldState {
	return interfaces.FactoryWorldState{
		PayloadLineage:   work.WorkPayloadLineageProjection{},
		WorkRequestsByID: make(map[string]interfaces.WorkRequestPayload),
		TerminalWorkByID: make(map[string]interfaces.FactoryTerminalWork),
		WorkItemsByID:    make(map[string]work.FactoryWorkItem),
	}
}

func primaryResultWorkItem(workID, workTypeName, stateName, name, placeID string) work.FactoryWorkItem {
	return work.FactoryWorkItem{
		ID:          workID,
		WorkTypeID:  workTypeName,
		State:       stateName,
		DisplayName: name,
		TraceID:     workID + "-trace",
		PlaceID:     placeID,
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: workID + "-content",
		}},
	}
}

func recordPrimaryResultSubmittedWork(
	state *interfaces.FactoryWorldState,
	tick int,
	requestID string,
	items ...work.FactoryWorkItem,
) {
	if state == nil {
		return
	}
	request := interfaces.WorkRequestPayload{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		WorkItems: append([]work.FactoryWorkItem(nil), items...),
	}
	state.WorkRequestsByID[requestID] = request
	for _, item := range items {
		state.PayloadLineage.RecordWorkRequestSnapshot(tick, requestID, item)
	}
}

func recordPrimaryResultDispatchOutput(
	state *interfaces.FactoryWorldState,
	tick int,
	dispatchID string,
	consumed []work.FactoryWorkItem,
	outputs ...work.FactoryWorkItem,
) {
	if state == nil {
		return
	}
	for _, item := range consumed {
		state.PayloadLineage.RecordConsumedInputSnapshot(dispatchID, item)
	}
	for i, item := range outputs {
		state.PayloadLineage.RecordDispatchOutputSnapshot(tick, dispatchID, consumed, item, i)
	}
}
