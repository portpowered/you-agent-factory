package execution_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIPetriDispatchUsageReachesDispatchList proves a completed Petri
// dispatch crosses a real provider command edge and the public dispatch-list
// endpoint without losing measured usage or inventing absent token facts.
func TestAPIPetriDispatchUsageReachesDispatchList(t *testing.T) {
	t.Parallel()
	acquireExecutionFixtureSlot(t)

	tests := []petriDispatchUsageCase{
		{
			name:         "provider token metadata is exposed",
			workerConfig: support.BuildModelWorkerConfig("codex", "test-model"),
			providerResult: platformprocess.CommandResult{
				Stdout: support.CodexSuccessStdoutWithUsage("Processed. COMPLETE", 12, 8),
			},
			wantInput:     int64Pointer(12),
			wantOutput:    int64Pointer(8),
			wantTotal:     int64Pointer(20),
			wantTokenKeys: true,
		},
		{
			name:         "missing provider token metadata stays absent",
			workerConfig: support.BuildModelWorkerConfig("claude", "test-model"),
			providerResult: platformprocess.CommandResult{
				Stdout: support.ClaudeSuccessStdout("Processed. COMPLETE"),
			},
			wantTokenKeys: false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			observation := runPetriDispatchUsage(t, test)
			assertPetriDispatchUsage(t, observation, test)
		})
	}
}

type petriDispatchUsageCase struct {
	name           string
	workerConfig   string
	providerResult platformprocess.CommandResult
	wantInput      *int64
	wantOutput     *int64
	wantTotal      *int64
	wantTokenKeys  bool
}

type petriDispatchUsageObservation struct {
	endpoint string
	body     []byte
	listed   factoryapi.ListFactorySessionDispatchesResponse
}

func runPetriDispatchUsage(t *testing.T, test petriDispatchUsageCase) petriDispatchUsageObservation {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", test.workerConfig)
	providerRunner := testutil.NewProviderCommandRunner(test.providerResult)
	observedRunner := &petriDispatchUsageRunner{delegate: providerRunner, calls: make(chan struct{}, 1)}
	session := openSharedExecutionSession(t, dir, sharedExecutionRouteConfig{provider: observedRunner})
	name := "rest-submit-petri-dispatch-usage"
	support.SubmitSessionWorkAt(t, session.fixture.baseURL, session.sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "REST Petri dispatch usage"},
	})
	if err := observedRunner.waitForCall(t.Context(), sharedExecutionFixtureTimeout); err != nil {
		t.Fatalf("wait for provider command edge: %v", err)
	}
	// The command edge proves the external effect returned; this bounded
	// public-session observation proves the runtime committed that result before
	// the session is closed and its durable dispatch projection is read.
	support.WaitForSessionTerminalStatus(t, session.fixture.baseURL, session.sessionID, sharedExecutionFixtureTimeout)
	// The provider edge is the deterministic completion witness for this
	// recording-backed projection. Close the live session before reading the
	// durable dispatch list, matching the original public route.
	session.close(t)
	listed, err := support.WaitForObservation(
		sharedExecutionFixtureTimeout,
		func() (factoryapi.ListFactorySessionDispatchesResponse, error) {
			return listFactorySessionDispatches(t, session.fixture.baseURL, session.sessionID), nil
		},
		func(listed factoryapi.ListFactorySessionDispatchesResponse) bool {
			return len(listed.Dispatches) == 1 && listed.Dispatches[0].Status == factoryapi.FactoryDispatchStatusCOMPLETED
		},
	)
	if err != nil {
		t.Fatalf("dispatch list did not reach COMPLETED: %v; last dispatches=%#v", err, listed.Dispatches)
	}
	endpoint := strings.TrimSuffix(session.fixture.baseURL, "/") + "/factory-sessions/" + session.sessionID + "/dispatches"
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatalf("read GET %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200: %s", endpoint, response.StatusCode, body)
	}
	var listedResponse factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(body, &listedResponse); err != nil {
		t.Fatalf("decode GET %s response: %v", endpoint, err)
	}
	return petriDispatchUsageObservation{endpoint: endpoint, body: body, listed: listedResponse}
}

type petriDispatchUsageRunner struct {
	delegate platformprocess.CommandRunner
	calls    chan struct{}
}

func (runner *petriDispatchUsageRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	result, err := runner.delegate.Run(ctx, request)
	if err == nil {
		select {
		case runner.calls <- struct{}{}:
		default:
		}
	}
	return result, err
}

func (runner *petriDispatchUsageRunner) waitForCall(ctx context.Context, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	select {
	case <-runner.calls:
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func assertPetriDispatchUsage(t *testing.T, observation petriDispatchUsageObservation, test petriDispatchUsageCase) {
	t.Helper()
	endpoint := observation.endpoint
	body := observation.body
	listed := observation.listed
	if len(listed.Dispatches) != 1 {
		t.Fatalf("GET %s dispatches = %#v, want one completed Petri dispatch", endpoint, listed.Dispatches)
	}
	dispatch := listed.Dispatches[0]
	if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if dispatch.DispatchKind != factoryapi.FactoryDispatchKindPETRITRANSITION {
		t.Fatalf("dispatch kind = %q, want PETRI_TRANSITION", dispatch.DispatchKind)
	}
	if dispatch.Usage == nil || dispatch.Usage.DurationMillis == nil {
		t.Fatalf("dispatch usage = %#v, want measured duration", dispatch.Usage)
	}
	if *dispatch.Usage.DurationMillis < 0 {
		t.Fatalf("dispatch duration = %d, want nonnegative", *dispatch.Usage.DurationMillis)
	}
	assertOptionalInt64(t, "inputTokens", dispatch.Usage.InputTokens, test.wantInput)
	assertOptionalInt64(t, "outputTokens", dispatch.Usage.OutputTokens, test.wantOutput)
	assertOptionalInt64(t, "totalTokens", dispatch.Usage.TotalTokens, test.wantTotal)
	if dispatch.Usage.CostUsd != nil {
		t.Fatalf("dispatch costUsd = %#v, want absent", dispatch.Usage.CostUsd)
	}
	assertUsageTokenFieldPresence(t, endpoint, body, test.wantTokenKeys)
}

func assertUsageTokenFieldPresence(t *testing.T, endpoint string, body []byte, wantPresent bool) {
	t.Helper()
	var envelope struct {
		Dispatches []map[string]json.RawMessage `json:"dispatches"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode raw GET %s response: %v", endpoint, err)
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Dispatches[0]["usage"], &usage); err != nil {
		t.Fatalf("decode raw dispatch usage: %v", err)
	}
	for _, key := range []string{"inputTokens", "outputTokens", "totalTokens"} {
		_, present := usage[key]
		if present != wantPresent {
			t.Fatalf("raw usage field %q present = %t, want %t: %s", key, present, wantPresent, body)
		}
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func assertOptionalInt64(t *testing.T, name string, got, want *int64) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("dispatch %s = %d, want absent", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("dispatch %s = %#v, want %d", name, got, *want)
	}
}
