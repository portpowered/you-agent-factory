package relationships

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	crossBatchPrerequisiteName = "prerequisite-a"
	crossBatchPrerequisiteID   = "work-prerequisite-a"
	crossBatchDependentName    = "dependent-b"
	crossBatchDependentID      = "work-dependent-b"
)

// testCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion proves the
// public two-batch flow: a dependency admitted while its target is active is
// visible at init without a dispatch, then releases once after the target's
// completion event. Git checkout propagation is a Workers contract and is not
// part of relationship admission or release.
func testCrossBatchDependsOnActivePrerequisiteReleasesAfterCompletion(
	t *testing.T,
	host *sharedRelationshipHost,
	gate *relationshipProviderGate,
) {
	factoryDir := scaffoldCrossBatchFactory(t)
	baseURL := host.URL()
	host.provider.register(t, factoryDir, sharedRelationshipGateProvider(gate))
	session, closeSession := openSharedRelationshipSession(t, baseURL, factoryDir)
	sessionID := session.Id

	executeCrossBatchSubmitForSessionOnServer(t, host.server, sessionID, crossBatchPrerequisiteBatchJSON())
	gate.WaitForArrival(t, 15*time.Second)
	assertCrossBatchWorkStateForSession(t, baseURL, sessionID, crossBatchPrerequisiteID, "processing", "active prerequisite")

	executeCrossBatchSubmitForSessionOnServer(t, host.server, sessionID, crossBatchDependentBatchJSON())
	assertCrossBatchWorkStateForSession(t, baseURL, sessionID, crossBatchDependentID, "init", "gated dependent")
	assertCrossBatchNoDependentStartDispatch(t, support.GetFactoryEventsForSessionAt(t, baseURL, sessionID))

	gate.Release()
	support.WaitForSessionTerminalStatus(t, baseURL, sessionID, 15*time.Second)
	listed := listRelationshipSessionWork(t, baseURL, sessionID)
	for _, workID := range []string{crossBatchPrerequisiteID, crossBatchDependentID} {
		if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
			t.Fatalf("Work %q did not reach complete: %#v", workID, listed)
		}
	}

	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
	prerequisiteCompleteSequence, dependentStartSequence := crossBatchDispatchOrdering(t, events)
	if dependentStartSequence <= prerequisiteCompleteSequence {
		t.Fatalf(
			"dependent start dispatch sequence = %d, want after prerequisite completion sequence %d",
			dependentStartSequence,
			prerequisiteCompleteSequence,
		)
	}
	closeSession()
	runSharedHostReuseProbe(t, baseURL)
}

func scaffoldCrossBatchFactory(t *testing.T) string {
	t.Helper()
	factoryDir := support.ScaffoldFactory(t, crossBatchDependencyFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig("codex", "test-model"))
	support.WriteWorkstationConfig(t, factoryDir, "start", "---\ntype: MODEL_WORKSTATION\n---\nAdvance cross-batch Work.\n")
	support.WriteWorkstationConfig(t, factoryDir, "finish", "---\ntype: MODEL_WORKSTATION\n---\nComplete cross-batch Work.\n")
	return factoryDir
}

func executeCrossBatchSubmitForSessionOnServer(
	t *testing.T,
	server *support.FunctionalAPIServer,
	sessionID string,
	batchJSON string,
) {
	t.Helper()
	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", server.URL(), "--session", sessionID, "--json", "submit", "batch", batchJSON,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = homeDir
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("submit batch to Factory Session %q: %v\nstdout:\n%s\nstderr:\n%s", sessionID, err, inputs.Stdout(), inputs.Stderr())
	}
}

func assertCrossBatchWorkStateForSession(
	t *testing.T,
	baseURL, sessionID, workID, state, description string,
) {
	t.Helper()
	wantPlace := support.WorkCustomerLocation("task", state)
	listed, err := support.WaitForObservation(
		15*time.Second,
		func() (factoryapi.ListWorkResponse, error) {
			return listRelationshipSessionWork(t, baseURL, sessionID), nil
		},
		func(listed factoryapi.ListWorkResponse) bool {
			return support.HasWorkAtCustomerState(listed, workID, wantPlace)
		},
	)
	if err != nil {
		t.Fatalf("observe %s: %v; listed=%#v", description, err, listed)
	}
}

func crossBatchDependencyFactoryConfig() map[string]any {
	return map[string]any{
		"name": "cross-batch-dependency-functional",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{
			{
				"name":      "start",
				"worker":    "worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "finish",
				"worker":    "worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func crossBatchPrerequisiteBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-prerequisite",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch prerequisite"}
		}]
	}`, crossBatchPrerequisiteName, crossBatchPrerequisiteID)
}

func crossBatchDependentBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": "cross-batch-dependent",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{
			"name": %q,
			"workId": %q,
			"workTypeName": "task",
			"payload": {"title": "Cross-batch dependent"}
		}],
		"relations": [{
			"type": "DEPENDS_ON",
			"sourceWorkName": %q,
			"targetWorkName": %q
		}]
	}`, crossBatchDependentName, crossBatchDependentID, crossBatchDependentName, crossBatchPrerequisiteName)
}

func assertCrossBatchNoDependentStartDispatch(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dependent-gate dispatch event: %v", err)
		}
		if payload.TransitionId == "start" && dispatchRequestIncludesWork(payload, crossBatchDependentID) {
			t.Fatalf("dependent Work received start dispatch before prerequisite completion at sequence %d", event.Context.Sequence)
		}
	}
}

func crossBatchDispatchOrdering(t *testing.T, events []factoryapi.FactoryEvent) (int, int) {
	t.Helper()
	prerequisiteCompleteSequence := -1
	dependentStartSequence := -1
	dependentStartCount := 0
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode prerequisite completion event: %v", err)
			}
			if payload.TransitionId == "finish" &&
				payload.Outcome == factoryapi.WorkOutcomeAccepted &&
				dispatchEventIncludesWork(event.Context.WorkIds, crossBatchPrerequisiteID) {
				prerequisiteCompleteSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode dependent start event: %v", err)
			}
			if payload.TransitionId == "start" && dispatchRequestIncludesWork(payload, crossBatchDependentID) {
				dependentStartCount++
				if dependentStartSequence < 0 {
					dependentStartSequence = event.Context.Sequence
				}
			}
		}
	}
	if prerequisiteCompleteSequence < 0 {
		t.Fatalf("prerequisite %q has no accepted finish dispatch", crossBatchPrerequisiteID)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent %q has no start dispatch", crossBatchDependentID)
	}
	if dependentStartCount != 1 {
		t.Fatalf("dependent %q start dispatch count = %d, want exactly one", crossBatchDependentID, dependentStartCount)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}
