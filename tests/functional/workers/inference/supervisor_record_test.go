package inference_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type supervisorRecordCase struct {
	name, decision, feedback, reason string
	skip, hiddenFinal, auth          bool
}

// Each cell owns its directory, command route, and explicit Factory Session.
func TestSupervisorRecordInspection(t *testing.T) {
	t.Parallel()
	for _, tc := range []supervisorRecordCase{
		{name: "F1 recovers successful final message", decision: "ACCEPTED", skip: true},
		{name: "F2 explains declared failure", decision: "FAILED", feedback: "admission was not confirmed", reason: "worker_declared_failure", skip: true},
		{name: "F3 keeps authentication failure", reason: "auth_failure", auth: true},
		{name: "F4 missing oversized final stays failed", reason: "internal_server_error", hiddenFinal: true},
		{name: "F5 redacts private failure feedback", decision: "FAILED", feedback: "admission blocked by supervisor-private-value", reason: "worker_declared_failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			exerciseSupervisorRecord(t, tc)
		})
	}
}

func supervisorRecordFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name":         "supervisor-record",
		"workTypes":    []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}}},
		"workers":      []map[string]string{{"name": "supervisor"}},
		"workstations": []any{map[string]any{"name": "supervise", "type": "AGENT_RUN", "worker": "supervisor", "outcomeFormat": "decision-envelope", "env": map[string]string{"SUPERVISOR_SECRET": "supervisor-private-value"}, "body": "Supervise the task.", "inputs": []map[string]string{{"workType": "task", "state": "init"}}, "outputs": []map[string]string{{"workType": "task", "state": "done"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}}},
	})
	support.WriteAgentConfig(t, dir, "supervisor", "---\nexecutorProvider: CODEX\ntype: AGENT_WORKER\nmodelProvider: CODEX\nmodel: gpt-5-codex\nskipPermissions: true\n---\nSupervise the task.\n")
	support.WriteWorkstationConfig(t, dir, "supervise", "---\ntype: AGENT_RUN\noutcomeFormat: decision-envelope\nenv:\n  SUPERVISOR_SECRET: supervisor-private-value\n---\nSupervise the task.\n")
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"supervision"}`))
	return dir
}

func assertSupervisorInference(t *testing.T, events []factoryapi.FactoryEvent, failed, skipped bool, discarded string) {
	t.Helper()
	found := false
	skipFound := false
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), discarded[:128]) {
			t.Fatal("published discarded record")
		}
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatal(err)
		}
		found = true
		assertSupervisorModelOutcome(t, payload, failed)

		if payload.Diagnostics != nil && payload.Diagnostics.Provider != nil && payload.Diagnostics.Provider.ResponseMetadata != nil {
			metadata := *payload.Diagnostics.Provider.ResponseMetadata
			skipFound = skipFound || metadata["inspection_records_skipped"] == "1"
		}
	}
	if !found || skipped && !skipFound {
		t.Fatalf("inference found=%v record skip found=%v", found, skipFound)
	}
}

func exerciseSupervisorRecord(t *testing.T, tc supervisorRecordCase) {
	t.Helper()
	dir := supervisorRecordFactory(t)
	final, err := json.Marshal(map[string]string{"decision": tc.decision, "feedback": tc.feedback, "output": "preserved supervisor output"})
	if err != nil {
		t.Fatal(err)
	}
	stream := support.CodexSuccessStdout(string(final))
	discarded := "discarded-private-tool-output-" + strings.Repeat("x", (1<<20)+128)
	if tc.skip {
		stream = append([]byte(`{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"`+discarded+`"}}`+"\n"), stream...)
	}
	if tc.hiddenFinal {
		stream = []byte(`{"type":"item.completed","item":{"type":"agent_message","text":"` + discarded + `"}}` + "\n")
	}
	command := platformprocess.CommandResult{Stdout: stream}
	if tc.auth {
		command = platformprocess.CommandResult{ExitCode: 1, Stderr: []byte(codexAuthFailureStderr)}
	}
	runner := testutil.NewProviderCommandRunner(command)
	if tc.hiddenFinal {
		for i := 0; i < 12; i++ {
			runner.Queue(command)
		}
	}
	_, listed, events := runSharedInferenceFactoryToCompletion(t, dir, sharedInferenceScenario{commandRunner: runner}, sharedInferenceScenarioTimeout)
	state := "task:done"
	if tc.reason != "" {
		state = "task:failed"
	}
	if support.CountWorkAtCustomerState(listed, state) != 1 {
		for _, w := range listed.Results {
			t.Logf("failure detail=%#v", w.FailureDetail)
		}
		t.Fatalf("work=%#v, want %s", listed, state)
	}
	if tc.hiddenFinal {
		// Record-limit dependency failures use the existing bounded retry policy.
		if runner.CallCount() < 1 {
			t.Fatal("provider was not invoked")
		}
	} else if runner.CallCount() != 1 {
		t.Fatalf("calls=%d, want one", runner.CallCount())
	}
	assertSupervisorDispatch(t, tc, listed, events)
	assertSupervisorInference(t, events, tc.reason != "", tc.skip || tc.hiddenFinal, discarded)
}

func assertSupervisorPrivateFailure(t *testing.T, events []factoryapi.FactoryEvent, listed factoryapi.ListWorkResponse, response *factoryapi.DispatchResponseEventPayload) {
	t.Helper()
	const want = "admission blocked by <redacted>"
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse || event.Type == factoryapi.FactoryEventTypeModelResponse || event.Type == factoryapi.FactoryEventTypeAgentRunResponse {
			raw, _ := json.Marshal(event)
			if strings.Contains(string(raw), "supervisor-private-value") {
				t.Fatalf("failure event leaked private value: %s", event.Type)
			}
		}
	}
	for _, w := range listed.Results {
		if w.FailureDetail == nil || w.FailureDetail.Message != want || string(w.FailureDetail.Reason) != "worker_declared_failure" {
			t.Fatalf("unsafe work failure: %#v", w.FailureDetail)
		}
	}
	if response.FailureDetail.Message != want || response.Feedback == nil || *response.Feedback != want {
		t.Fatalf("unsafe detail=%#v", response.FailureDetail)
	}
}

func assertSupervisorDispatch(t *testing.T, tc supervisorRecordCase, listed factoryapi.ListWorkResponse, events []factoryapi.FactoryEvent) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 || dispatches[len(dispatches)-1].Response == nil {
		t.Fatalf("dispatches=%#v", dispatches)
	}
	response := dispatches[len(dispatches)-1].Response
	if tc.reason == "" {
		if response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("response=%#v", response)
		}
	} else if response.Outcome != factoryapi.WorkOutcomeFailed || response.FailureDetail == nil || string(response.FailureDetail.Reason) != tc.reason {
		t.Fatalf("response=%#v detail=%#v", response, response.FailureDetail)
	}
	assertSupervisorExplanation(t, tc, response)
	if tc.name == "F5 redacts private failure feedback" {
		assertSupervisorPrivateFailure(t, events, listed, response)
	}
}

func assertSupervisorModelOutcome(t *testing.T, payload factoryapi.ModelResponseEventPayload, failed bool) {
	t.Helper()
	want := factoryapi.InferenceOutcomeSucceeded
	if failed {
		want = factoryapi.InferenceOutcomeFailed
	}
	if payload.Outcome != want {
		t.Fatalf("inference=%#v", payload)
	}
	if !failed {
		raw, err := json.Marshal(payload.OutputContent)
		if err != nil || !strings.Contains(string(raw), "preserved supervisor output") {
			t.Fatalf("model output=%s err=%v", raw, err)
		}
	}
}

func assertSupervisorExplanation(t *testing.T, tc supervisorRecordCase, response *factoryapi.DispatchResponseEventPayload) {
	t.Helper()
	if tc.decision != "" && (response.Output == nil || *response.Output != "preserved supervisor output") {
		t.Fatalf("output=%v", response.Output)
	}
	if tc.hiddenFinal && !strings.Contains(response.FailureDetail.Message, "record limit") {
		t.Fatalf("missing record-limit explanation: %#v", response.FailureDetail)
	}
	if tc.auth && !strings.Contains(strings.ToLower(response.FailureDetail.Message), "auth") {
		t.Fatalf("missing auth explanation: %#v", response.FailureDetail)
	}
	if tc.name == "F2 explains declared failure" && response.FailureDetail.Message != tc.feedback {
		t.Fatalf("detail=%#v", response.FailureDetail)
	}
}
