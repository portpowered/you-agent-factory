package mock

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	mockedWorkerName      = "mocked-worker"
	realWorkerName        = "real-worker"
	mockedWorkstationName = "mock-process"
	realWorkstationName   = "real-process"
	mockedWorkType        = "mock-task"
	realWorkType          = "real-task"
	mockedWorkID          = "named-mock-replacement-mocked-work"
	realWorkID            = "named-mock-replacement-real-work"
	injectedProviderOutput = "injected-real-worker-output COMPLETE"
)

// TestMockWorkersReplaceOnlyNamedChildren proves a partial --with-mock-workers
// config replaces only the named workers while unmatched workers execute through
// the real or injected provider path when unmatchedDispatchPolicy is passthrough.
func TestMockWorkersReplaceOnlyNamedChildren(t *testing.T) {
	dir := scaffoldNamedReplacementFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(injectedProviderOutput)},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:        dir,
		MockWorkersConfig: partialNamedMockWorkersConfig(),
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	for placeID, want := range map[string]int{
		support.WorkCustomerLocation(mockedWorkType, "done"): 1,
		support.WorkCustomerLocation(realWorkType, "done"):   1,
		support.WorkCustomerLocation(mockedWorkType, "init"): 0,
		support.WorkCustomerLocation(realWorkType, "init"):   0,
		support.WorkCustomerLocation(mockedWorkType, "failed"): 0,
		support.WorkCustomerLocation(realWorkType, "failed"):   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1 passthrough dispatch for unnamed worker", runner.CallCount())
	}

	observations := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	mockObservation := dispatchObservationByTransition(t, observations, mockedWorkstationName)
	realObservation := dispatchObservationByTransition(t, observations, realWorkstationName)

	assertMockAcceptedDispatch(t, mockObservation)
	assertInjectedProviderDispatch(t, realObservation)
}

func scaffoldNamedReplacementFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{
			namedReplacementWorkType(mockedWorkType),
			namedReplacementWorkType(realWorkType),
		},
		"workers": []map[string]string{
			{"name": mockedWorkerName},
			{"name": realWorkerName},
		},
		"workstations": []map[string]any{
			{
				"name":      mockedWorkstationName,
				"worker":    mockedWorkerName,
				"inputs":    []map[string]string{{"workType": mockedWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": mockedWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": mockedWorkType, "state": "failed"}},
			},
			{
				"name":      realWorkstationName,
				"worker":    realWorkerName,
				"inputs":    []map[string]string{{"workType": realWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": realWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": realWorkType, "state": "failed"}},
			},
		},
	})

	modelWorker := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	support.WriteAgentConfig(t, dir, mockedWorkerName, modelWorker)
	support.WriteAgentConfig(t, dir, realWorkerName, modelWorker)

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     mockedWorkID,
		WorkTypeID: mockedWorkType,
		TraceID:    "named-mock-replacement-mocked-trace",
		Payload:    []byte(`{"title":"named mock replacement mocked"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     realWorkID,
		WorkTypeID: realWorkType,
		TraceID:    "named-mock-replacement-real-trace",
		Payload:    []byte(`{"title":"named mock replacement real"}`),
	})
	return dir
}

func namedReplacementWorkType(name string) map[string]any {
	return map[string]any{
		"name": name,
		"states": []map[string]string{
			{"name": "init", "type": "INITIAL"},
			{"name": "done", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}
}

func partialNamedMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      mockedWorkerName,
			WorkstationName: mockedWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
}

func dispatchObservationByTransition(
	t *testing.T,
	observations []support.DispatchEventObservation,
	transitionID string,
) support.DispatchEventObservation {
	t.Helper()

	for _, observation := range observations {
		if observation.Request.TransitionId == transitionID {
			return observation
		}
	}
	t.Fatalf("dispatch observation for transition %q not found in %#v", transitionID, observations)
	return support.DispatchEventObservation{}
}

func assertMockAcceptedDispatch(t *testing.T, observation support.DispatchEventObservation) {
	t.Helper()

	if observation.Response == nil {
		t.Fatalf("mock-process dispatch response missing: %#v", observation)
	}
	if observation.Response.Outcome != factoryapi.WorkOutcome(workerexecution.OutcomeAccepted) {
		t.Fatalf(
			"mock-process outcome = %s, want %s",
			observation.Response.Outcome,
			workerexecution.OutcomeAccepted,
		)
	}
	output := stringPointerValue(observation.Response.Output)
	if !strings.Contains(output, "mock worker accepted") {
		t.Fatalf("mock-process output = %q, want configured mock accept output", output)
	}
}

func assertInjectedProviderDispatch(t *testing.T, observation support.DispatchEventObservation) {
	t.Helper()

	if observation.Response == nil {
		t.Fatalf("real-process dispatch response missing: %#v", observation)
	}
	if observation.Response.Outcome != factoryapi.WorkOutcome(workerexecution.OutcomeAccepted) {
		t.Fatalf(
			"real-process outcome = %s, want %s",
			observation.Response.Outcome,
			workerexecution.OutcomeAccepted,
		)
	}
	output := stringPointerValue(observation.Response.Output)
	if !strings.Contains(output, injectedProviderOutput) {
		t.Fatalf("real-process output = %q, want injected provider output %q", output, injectedProviderOutput)
	}
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
