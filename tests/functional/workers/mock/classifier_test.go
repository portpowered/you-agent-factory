package mock

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	scriptClassifierWorkType       = "review-task"
	scriptClassifierWorker         = "script-classifier"
	scriptClassifierWorkstation    = "classifier"
	scriptClassifierWorkID         = "script-classifier-work"
	scriptClassifierLabel          = "needs_review"
	scriptClassifierDiagnosticLine = "checking payload"
)

// testScriptWorkerClassifierRoutesWithoutModelCalls proves the workers/mock
// ownership cell can route a script-backed classifier through the canonical
// process and Factory Event flow without invoking a model provider.
func testScriptWorkerClassifierRoutesWithoutModelCalls(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldScriptClassifierFactory(t)

	scriptRunner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(scriptClassifierDiagnosticLine + "\n  " + scriptClassifierLabel + "  \n\n"),
		Stderr: []byte("script diagnostic"),
	})
	providerRunner := testutil.NewProviderCommandRunner()

	fixture.useCommandRunnersFor(t, dir, providerRunner, scriptRunner)
	session := fixture.openSession(t, dir)
	listed, events := session.terminalObservations(t, 20*time.Second)
	defer session.closeAndAssertGone(t)
	for placeID, want := range map[string]int{
		support.WorkCustomerLocation(scriptClassifierWorkType, "done"):   1,
		support.WorkCustomerLocation(scriptClassifierWorkType, "init"):   0,
		support.WorkCustomerLocation(scriptClassifierWorkType, "failed"): 0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Fatalf("%s token count = %d, want %d; listed=%#v", placeID, got, want, listed)
		}
	}
	if scriptRunner.CallCount() != 1 {
		t.Fatalf("script command runner calls = %d, want exactly one classifier dispatch", scriptRunner.CallCount())
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want zero model-provider invocations", providerRunner.CallCount())
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	classifierDispatches := make([]support.DispatchEventObservation, 0, 1)
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == scriptClassifierWorkstation {
			classifierDispatches = append(classifierDispatches, dispatch)
		}
	}
	if len(classifierDispatches) != 1 {
		t.Fatalf(
			"classifier dispatch count = %d, want one accepted dispatch without retries; dispatches=%#v",
			len(classifierDispatches),
			classifierDispatches,
		)
	}
	response := classifierDispatches[0].Response
	if response == nil {
		t.Fatal("classifier dispatch response missing")
	}
	if response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("classifier outcome = %s, want ACCEPTED", response.Outcome)
	}
	if support.StringPointerValue(response.Output) != scriptClassifierLabel {
		t.Fatalf("classifier output = %q, want %q", support.StringPointerValue(response.Output), scriptClassifierLabel)
	}
	if support.StringPointerValue(response.SelectedClassificationLabel) != scriptClassifierLabel {
		t.Fatalf(
			"selected classification label = %q, want %q",
			support.StringPointerValue(response.SelectedClassificationLabel),
			scriptClassifierLabel,
		)
	}
	if response.Error != nil || response.ProviderFailure != nil {
		t.Fatalf("classifier response = %#v, want no failure or circuit-breaker sequence", response)
	}

	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeInferenceRequest ||
			event.Type == factoryapi.FactoryEventTypeInferenceResponse {
			t.Fatalf("script classifier emitted model inference event: %#v", event)
		}
	}
}

func scaffoldScriptClassifierFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{{
			"name": scriptClassifierWorkType,
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": scriptClassifierWorker}},
		"workstations": []map[string]any{{
			"name":   scriptClassifierWorkstation,
			"type":   "CLASSIFIER_WORKSTATION",
			"worker": scriptClassifierWorker,
			"inputs": []map[string]string{{"workType": scriptClassifierWorkType, "state": "init"}},
			"classificationRoutes": []map[string]any{{
				"label":   scriptClassifierLabel,
				"outputs": []map[string]string{{"workType": scriptClassifierWorkType, "state": "done"}},
			}},
			"onFailure": []map[string]string{{"workType": scriptClassifierWorkType, "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, scriptClassifierWorker, "---\n"+
		"type: SCRIPT_WORKER\n"+
		"command: authored-classifier-command\n"+
		"args:\n"+
		"  - authored-argument\n"+
		"---\n")
	support.WriteWorkstationConfig(t, dir, scriptClassifierWorkstation, "---\n"+
		"type: CLASSIFIER_WORKSTATION\n"+
		"---\n"+
		"Classify the review task.\n")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     scriptClassifierWorkID,
		WorkTypeID: scriptClassifierWorkType,
		TraceID:    "script-classifier-trace",
		Payload:    []byte("classifier payload"),
	})
	return dir
}
