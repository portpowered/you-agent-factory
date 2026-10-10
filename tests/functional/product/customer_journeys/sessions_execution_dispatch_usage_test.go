package customer_journeys_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestAPIPetriDispatchUsageReachesCanonicalStream proves a completed Petri
// dispatch crosses a real provider command edge and its live canonical event
// stream without losing measured usage or inventing absent token facts.
func TestAPIPetriDispatchUsageReachesCanonicalStream(t *testing.T) {
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
			response, endpoint := runPetriDispatchUsage(t, test)
			assertPetriDispatchUsage(t, response, endpoint, test)
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

func runPetriDispatchUsage(t *testing.T, test petriDispatchUsageCase) (factoryapi.FactoryEvent, string) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", test.workerConfig)
	const sessionID = "00000000-0000-4000-8000-000000000001"
	providerRunner := testutil.NewProviderCommandRunner(test.providerResult)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			FactorySessionIDGenerator: func() string { return sessionID },
			ProviderCommandRunner:     providerRunner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	if opened.Session == nil || opened.Session.Id != sessionID {
		t.Fatalf("opened session = %#v, want id %q", opened.Session, sessionID)
	}
	name := "rest-submit-petri-dispatch-usage"
	support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "REST Petri dispatch usage"},
	})
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 10*time.Second)
	events := support.GetFactoryEventsForSessionAt(t, server.URL(), sessionID)
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			return event, server.URL() + "/factory-sessions/" + sessionID + "/dispatches"
		}
	}
	t.Fatalf("live Factory Session %q emitted no DISPATCH_RESPONSE", sessionID)
	return factoryapi.FactoryEvent{}, ""
}

func assertPetriDispatchUsage(t *testing.T, event factoryapi.FactoryEvent, endpoint string, test petriDispatchUsageCase) {
	t.Helper()
	dispatch, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE: %v", err)
	}
	if dispatch.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("dispatch outcome = %q, want ACCEPTED", dispatch.Outcome)
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
	assertUsageTokenFieldPresence(t, event.Payload, test.wantTokenKeys)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("live Petri dispatch history status = %d, want 404: %s", response.StatusCode, body)
	}
}

func assertUsageTokenFieldPresence(t *testing.T, payload factoryapi.FactoryEvent_Payload, wantPresent bool) {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal dispatch response payload: %v", err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode dispatch response payload: %v", err)
	}
	var usage map[string]json.RawMessage
	if err := json.Unmarshal(envelope["usage"], &usage); err != nil {
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
