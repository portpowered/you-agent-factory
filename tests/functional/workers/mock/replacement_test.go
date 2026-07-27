package mock

import (
	"context"
	"os"
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
	unknownMockWorkerName = "ghost-worker"
	invalidMockRunType    = "bogus"

	mockedWorkerName      = "mocked-worker"
	realWorkerName        = "real-worker"
	mockedWorkstationName = "mock-process"
	realWorkstationName   = "real-process"
	mockedWorkType        = "mock-task"
	realWorkType          = "real-task"
	mockedWorkID          = "named-mock-replacement-mocked-work"
	realWorkID            = "named-mock-replacement-real-work"
	injectedProviderOutput = "injected-real-worker-output COMPLETE"

	rejectWorkerName         = "reject-worker"
	rejectWorkstationName    = "reject-process"
	rejectWorkType           = "reject-task"
	rejectWorkID             = "mock-reject-work"
	configuredRejectStdout   = "configured reject stdout"
	configuredRejectStderr   = "configured reject stderr"
	stableUnknownProviderErr = "provider error: unknown: Codex reported a terminal error."
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

// TestUnknownWorkerOverrideFailsActionably proves an unknown or invalid
// --with-mock-workers override fails before dispatch with a stable,
// customer-visible diagnostic instead of silently accepting the bad override.
func TestUnknownWorkerOverrideFailsActionably(t *testing.T) {
	dir := scaffoldNamedReplacementFactory(t)

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
		{
			name: "unknown nested override field",
			payload: `{
				"mockWorkers": [
					{
						"workerName": "` + unknownMockWorkerName + `",
						"runType": "accept",
						"unexpectedNested": true
					}
				]
			}`,
			wantNeedle:    "unexpectednested",
			wantSecondary: "unknown field",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := testutil.NewProviderCommandRunner(
				platformprocess.CommandResult{Stdout: []byte(injectedProviderOutput)},
			)
			mockWorkersPath := writeRawMockWorkersConfig(t, tc.payload)
			diagnostic := executeRunWithMockWorkersExpectingFailure(
				t,
				dir,
				mockWorkersPath,
				serviceedges.Edges{ProviderCommandRunner: runner},
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

// TestMockWorkerFailureReturnsStablePublicFailure proves configured mock
// rejection yields a stable public failed Work / Factory Event outcome without
// live provider credentials.
func TestMockWorkerFailureReturnsStablePublicFailure(t *testing.T) {
	dir := scaffoldMockRejectFactory(t)
	exitCode := 7
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("live provider should not run")},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:        dir,
		MockWorkersConfig: rejectingMockWorkersConfig(exitCode),
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	defer server.Stop(t)

	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	listed := support.ListDefaultSessionWork(t, server.URL())
	for placeID, want := range map[string]int{
		support.WorkCustomerLocation(rejectWorkType, "failed"): 1,
		support.WorkCustomerLocation(rejectWorkType, "init"):   0,
		support.WorkCustomerLocation(rejectWorkType, "done"):   0,
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

	observation := dispatchObservationByTransition(
		t,
		support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)),
		rejectWorkstationName,
	)
	assertStableMockRejectDispatch(t, observation)
}

func executeRunWithMockWorkersExpectingFailure(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	edges serviceedges.Edges,
) string {
	t.Helper()

	process := support.BuildProcess(t, edges)
	inputs := support.FakeInputs(context.Background(), []string{
		"you",
		"run",
		"--dir", factoryDir,
		"--continuously",
		"--with-server",
		"--quiet",
		"--no-record",
		"--with-mock-workers", mockWorkersPath,
	})
	inputs.WorkingDirectory = factoryDir

	err := process.Execute(inputs.Input)
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

func scaffoldMockRejectFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{
			namedReplacementWorkType(rejectWorkType),
		},
		"workers": []map[string]string{
			{"name": rejectWorkerName},
		},
		"workstations": []map[string]any{
			{
				"name":      rejectWorkstationName,
				"worker":    rejectWorkerName,
				"inputs":    []map[string]string{{"workType": rejectWorkType, "state": "init"}},
				"outputs":   []map[string]string{{"workType": rejectWorkType, "state": "done"}},
				"onFailure": []map[string]string{{"workType": rejectWorkType, "state": "failed"}},
			},
		},
	})

	modelWorker := support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex")
	support.WriteAgentConfig(t, dir, rejectWorkerName, modelWorker)

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     rejectWorkID,
		WorkTypeID: rejectWorkType,
		TraceID:    "mock-reject-trace",
		Payload:    []byte(`{"title":"configured mock reject"}`),
	})
	return dir
}

func rejectingMockWorkersConfig(exitCode int) *workers.MockWorkersConfig {
	code := exitCode
	return &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      rejectWorkerName,
			WorkstationName: rejectWorkstationName,
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stdout:   configuredRejectStdout,
				Stderr:   configuredRejectStderr,
				ExitCode: &code,
			},
		}},
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
		string(*payload.ProviderFailure.Type) != string(workerexecution.WorkFailureTypeUnknown) {
		t.Fatalf(
			"reject-process provider failure = %#v, want stable unknown provider failure",
			payload.ProviderFailure,
		)
	}
	if payload.Error == nil {
		t.Fatal("reject-process dispatch error missing from public response")
	}
	errorText := *payload.Error
	if !strings.Contains(errorText, stableUnknownProviderErr) {
		t.Fatalf(
			"reject-process error = %q, want stable public unknown provider failure",
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
