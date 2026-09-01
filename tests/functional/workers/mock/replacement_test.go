package mock

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	unknownMockWorkerName = "ghost-worker"
	invalidMockRunType    = "bogus"

	mockedWorkerName       = "mocked-worker"
	realWorkerName         = "real-worker"
	mockedWorkstationName  = "mock-process"
	realWorkstationName    = "real-process"
	mockedWorkType         = "mock-task"
	realWorkType           = "real-task"
	mockedWorkID           = "named-mock-replacement-mocked-work"
	realWorkID             = "named-mock-replacement-real-work"
	injectedProviderOutput = `{"type":"item.completed","item":{"id":"message-final","type":"agent_message","text":"injected-real-worker-output COMPLETE"}}` + "\n"

	rejectWorkerName         = "reject-worker"
	rejectWorkstationName    = "reject-process"
	rejectWorkType           = "reject-task"
	rejectWorkID             = "mock-reject-work"
	configuredRejectStdout   = "configured reject stdout"
	configuredRejectStderr   = "configured reject stderr"
	stableProviderRefusalErr = "provider error: permanent_bad_request: provider rejected the execution request"
)

// testMockWorkersReplaceOnlyNamedChildren proves a partial --with-mock-workers
// config replaces only the named workers while unmatched workers execute through
// the real or injected provider path when unmatchedDispatchPolicy is passthrough.
func testMockWorkersReplaceOnlyNamedChildren(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldNamedReplacementFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(injectedProviderOutput)},
	)

	fixture.useCommandRunnersFor(t, dir, runner, nil)
	session := fixture.openSession(t, dir)
	listed, events := session.terminalObservations(t, 20*time.Second)
	defer session.closeAndAssertGone(t)
	for placeID, want := range map[string]int{
		support.WorkCustomerLocation(mockedWorkType, "done"):   1,
		support.WorkCustomerLocation(realWorkType, "done"):     1,
		support.WorkCustomerLocation(mockedWorkType, "init"):   0,
		support.WorkCustomerLocation(realWorkType, "init"):     0,
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

	observations := support.ObserveDispatchEvents(t, events)
	mockObservation := dispatchObservationByTransition(t, observations, mockedWorkstationName)
	realObservation := dispatchObservationByTransition(t, observations, realWorkstationName)

	assertMockAcceptedDispatch(t, mockObservation)
	assertInjectedProviderDispatch(t, realObservation)
}

// testUnknownWorkerOverrideFailsActionably proves an invalid
// --with-mock-workers override fails before dispatch with a stable,
// customer-visible diagnostic instead of silently accepting the bad override.
func testUnknownWorkerOverrideFailsActionably(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldNamedReplacementFactory(t)
	support.ClearSeedInputs(t, dir)

	tests := []struct {
		name          string
		payload       string
		wantNeedle    string
		wantSecondary string
	}{
		{
			name: "invalid runType in override entry",
			payload: `{
				"mockWorkers": [
					{
						"workerName": "` + unknownMockWorkerName + `",
						"runType": "` + invalidMockRunType + `"
					}
				]
			}`,
			wantNeedle:    invalidMockRunType,
			wantSecondary: `runtype must be one of "accept", "script", or "reject"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := testutil.NewProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte(injectedProviderOutput)},
			)
			fixture.useCommandRunnersFor(t, dir, runner, nil)
			mockWorkersPath := writeRawMockWorkersConfig(t, tc.payload)
			diagnostic := executeRunWithMockWorkersExpectingFailure(
				t,
				fixture,
				dir,
				mockWorkersPath,
			)

			lowDiagnostic := strings.ToLower(diagnostic)
			if !strings.Contains(lowDiagnostic, strings.ToLower(tc.wantNeedle)) {
				t.Fatalf("diagnostic = %q, want override identifier %q", diagnostic, tc.wantNeedle)
			}
			if !strings.Contains(lowDiagnostic, strings.ToLower(tc.wantSecondary)) {
				t.Fatalf(
					"diagnostic = %q, want actionable context containing %q",
					diagnostic,
					tc.wantSecondary,
				)
			}
			if runner.CallCount() != 0 {
				t.Fatalf(
					"provider command runner calls = %d after rejected override, want 0 pre-dispatch rejection",
					runner.CallCount(),
				)
			}
		})
	}
}

// testFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved proves compatible future fields do not alter mock dispatch behavior.
func testFutureMockWorkerFieldsAreIgnoredAndDispatchBehaviorIsPreserved(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldFutureReplacementFactory(t)
	fixture.useCommandRunnersFor(t, dir, nil, nil)
	session := fixture.openSession(t, dir)
	listed, events := session.terminalObservations(t, 20*time.Second)
	defer session.closeAndAssertGone(t)
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation(futureMockWorkType, "done")); got != 1 {
		t.Fatalf("mocked work done count = %d, want 1", got)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation(futureMockWorkType, "failed")); got != 0 {
		t.Fatalf("mocked work failed count = %d, want 0", got)
	}

	observation := dispatchObservationByTransition(
		t,
		support.ObserveDispatchEvents(t, events),
		futureMockWorkstationName,
	)
	assertMockAcceptedDispatch(t, observation)
}

// testMockWorkerFailureReturnsStablePublicFailure proves configured mock
// rejection yields a stable public failed Work / Factory Event outcome without
// live provider credentials.
func testMockWorkerFailureReturnsStablePublicFailure(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := scaffoldMockRejectFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("live provider should not run")},
	)

	fixture.useCommandRunnersFor(t, dir, runner, nil)
	session := fixture.openSession(t, dir)
	gateWaitStarted := time.Now()
	fixture.gate.WaitForArrival(t, 5*time.Second)
	listed, events := session.terminalObservations(t, 20*time.Second)
	if elapsed := time.Since(gateWaitStarted); elapsed < gateTimeoutDuration {
		t.Fatalf("mock-worker gate completed after %s, want at least configured timeout %s", elapsed, gateTimeoutDuration)
	}
	for placeID, want := range map[string]int{
		support.WorkCustomerLocation(rejectWorkType, "failed"):      1,
		support.WorkCustomerLocation(rejectWorkType, "init"):        0,
		support.WorkCustomerLocation(rejectWorkType, "done"):        0,
		support.WorkCustomerLocation(gateTimeoutWorkType, "failed"): 1,
		support.WorkCustomerLocation(gateTimeoutWorkType, "init"):   0,
		support.WorkCustomerLocation(gateTimeoutWorkType, "done"):   0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if runner.CallCount() != 0 {
		t.Fatalf(
			"provider command runner calls = %d after mock reject, want 0 without live provider credentials",
			runner.CallCount(),
		)
	}

	observations := support.ObserveDispatchEvents(t, events)
	var rejectDispatches, timeoutDispatches int
	for _, observation := range observations {
		switch observation.Request.TransitionId {
		case rejectWorkstationName:
			rejectDispatches++
			assertStableMockRejectDispatch(t, observation)
		case gateTimeoutWorkstation:
			timeoutDispatches++
			if !support.DispatchObservationIncludesWork(observation, gateTimeoutWorkID) {
				t.Fatalf("gate-timeout dispatch = %#v, want work correlation %q", observation, gateTimeoutWorkID)
			}
			assertMockGateTimeoutDispatch(t, observation)
		default:
			t.Fatalf("unexpected mock failure dispatch transition = %q", observation.Request.TransitionId)
		}
	}
	if rejectDispatches != 1 || timeoutDispatches == 0 {
		t.Fatalf(
			"mock failure dispatch counts = reject %d, gate-timeout %d; want one reject and at least one configured timeout",
			rejectDispatches,
			timeoutDispatches,
		)
	}

	// The next table row opens a new explicit session and completes a normal
	// mock dispatch. Closing this failed session first makes that subsequent
	// success an observable shared-host usability proof.
	session.closeAndAssertGone(t)
}

func executeRunWithMockWorkersExpectingFailure(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
	factoryDir string,
	mockWorkersPath string,
) string {
	t.Helper()

	session := fixture.openSession(t, factoryDir)
	defer session.closeAndAssertGone(t)
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"run",
		"--dir", factoryDir,
		"--quiet",
		"--no-record",
		"--with-mock-workers", mockWorkersPath,
	})
	inputs.Input.WorkingDirectory = factoryDir

	err := fixture.server.Execute(t, inputs.Input)
	if err == nil {
		t.Fatalf(
			"expected invalid mock-worker override to fail before dispatch; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := strings.ToLower(strings.Join([]string{
		err.Error(),
		inputs.Stderr(),
		inputs.Stdout(),
	}, "\n"))
	if strings.TrimSpace(diagnostic) == "" {
		t.Fatal("expected customer-visible diagnostic for invalid mock-worker override")
	}
	return diagnostic
}

func writeRawMockWorkersConfig(t *testing.T, payload string) string {
	t.Helper()

	path := t.TempDir() + "/mock-workers.json"
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("write mock workers config: %v", err)
	}
	return path
}

func scaffoldNamedReplacementFactory(t *testing.T) string {
	t.Helper()
	return scaffoldReplacementFactory(t, replacementFactorySpec{
		mockedWorker:      mockedWorkerName,
		realWorker:        realWorkerName,
		mockedWorkstation: mockedWorkstationName,
		realWorkstation:   realWorkstationName,
		mockedWorkType:    mockedWorkType,
		realWorkType:      realWorkType,
		mockedWorkID:      mockedWorkID,
		realWorkID:        realWorkID,
	}, true)
}

func scaffoldFutureReplacementFactory(t *testing.T) string {
	t.Helper()
	return scaffoldReplacementFactory(t, replacementFactorySpec{
		mockedWorker:      futureMockWorkerName,
		realWorker:        "future-real-worker",
		mockedWorkstation: futureMockWorkstationName,
		realWorkstation:   "future-real-process",
		mockedWorkType:    futureMockWorkType,
		realWorkType:      "future-real-task",
		mockedWorkID:      futureMockWorkID,
		realWorkID:        "future-real-work",
	}, false)
}

type replacementFactorySpec struct {
	mockedWorker      string
	realWorker        string
	mockedWorkstation string
	realWorkstation   string
	mockedWorkType    string
	realWorkType      string
	mockedWorkID      string
	realWorkID        string
}

func scaffoldReplacementFactory(
	t *testing.T,
	spec replacementFactorySpec,
	seedReal bool,
) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{
			namedReplacementWorkType(spec.mockedWorkType),
			namedReplacementWorkType(spec.realWorkType),
		},
		"workers": []map[string]string{
			{"name": spec.mockedWorker},
			{"name": spec.realWorker},
		},
		"workstations": []map[string]any{
			{
				"name":      spec.mockedWorkstation,
				"worker":    spec.mockedWorker,
				"inputs":    []map[string]string{{"workType": spec.mockedWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": spec.mockedWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": spec.mockedWorkType, "state": "failed"}},
			},
			{
				"name":      spec.realWorkstation,
				"worker":    spec.realWorker,
				"inputs":    []map[string]string{{"workType": spec.realWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": spec.realWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": spec.realWorkType, "state": "failed"}},
			},
		},
	})

	modelWorker := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	support.WriteAgentConfig(t, dir, spec.mockedWorker, modelWorker)
	support.WriteAgentConfig(t, dir, spec.realWorker, modelWorker)

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     spec.mockedWorkID,
		WorkTypeID: spec.mockedWorkType,
		TraceID:    spec.mockedWorkID + "-trace",
		Payload:    []byte(`{"title":"named mock replacement mocked"}`),
	})
	if seedReal {
		testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
			WorkID:     spec.realWorkID,
			WorkTypeID: spec.realWorkType,
			TraceID:    spec.realWorkID + "-trace",
			Payload:    []byte(`{"title":"named mock replacement real"}`),
		})
	}
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

func scaffoldMockRejectFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{
			namedReplacementWorkType(rejectWorkType),
			namedReplacementWorkType(gateTimeoutWorkType),
		},
		"workers": []map[string]string{
			{"name": rejectWorkerName},
			{"name": gateTimeoutWorker},
		},
		"workstations": []map[string]any{
			{
				"name":      rejectWorkstationName,
				"worker":    rejectWorkerName,
				"inputs":    []map[string]string{{"workType": rejectWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": rejectWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": rejectWorkType, "state": "failed"}},
			},
			{
				"name":      gateTimeoutWorkstation,
				"worker":    gateTimeoutWorker,
				"inputs":    []map[string]string{{"workType": gateTimeoutWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": gateTimeoutWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": gateTimeoutWorkType, "state": "failed"}},
			},
		},
	})

	modelWorker := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	support.WriteAgentConfig(t, dir, rejectWorkerName, modelWorker)
	support.WriteAgentConfig(t, dir, gateTimeoutWorker, modelWorker)

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     rejectWorkID,
		WorkTypeID: rejectWorkType,
		TraceID:    "mock-reject-trace",
		Payload:    []byte(`{"title":"configured mock reject"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     gateTimeoutWorkID,
		WorkTypeID: gateTimeoutWorkType,
		TraceID:    gateTimeoutWorkID + "-trace",
		Payload:    []byte(`{"title":"configured mock gate timeout"}`),
	})
	return dir
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

func assertStableMockRejectDispatch(t *testing.T, observation support.DispatchEventObservation) {
	t.Helper()

	if observation.Response == nil {
		t.Fatalf("reject-process dispatch response missing: %#v", observation)
	}
	payload := observation.Response
	if payload.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf(
			"reject-process outcome = %s, want %s",
			payload.Outcome,
			factoryapi.WorkOutcomeFailed,
		)
	}
	if payload.ProviderFailure == nil ||
		payload.ProviderFailure.Type == nil ||
		string(*payload.ProviderFailure.Type) != string(workerexecution.WorkFailureTypePermanentBadRequest) {
		t.Fatalf(
			"reject-process provider failure = %#v, want neutral terminal provider refusal",
			payload.ProviderFailure,
		)
	}
	if payload.Error == nil {
		t.Fatal("reject-process dispatch error missing from public response")
	}
	errorText := *payload.Error
	if !strings.Contains(errorText, stableProviderRefusalErr) {
		t.Fatalf(
			"reject-process error = %q, want neutral terminal provider refusal",
			errorText,
		)
	}
	for _, leaked := range []string{configuredRejectStdout, configuredRejectStderr} {
		if strings.Contains(errorText, leaked) {
			t.Fatalf(
				"reject-process error = %q, want customer-safe error without configured command output %q",
				errorText,
				leaked,
			)
		}
	}
	output := stringPointerValue(payload.Output)
	for _, leaked := range []string{configuredRejectStdout, configuredRejectStderr} {
		if strings.Contains(output, leaked) {
			t.Fatalf(
				"reject-process output = %q, want public surface without configured command output %q",
				output,
				leaked,
			)
		}
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
	if !strings.Contains(output, "injected-real-worker-output COMPLETE") {
		t.Fatalf("real-process output = %q, want injected provider message content", output)
	}
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
