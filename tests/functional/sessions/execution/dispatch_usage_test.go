package execution_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
			endpoint := runPetriDispatchUsage(t, test)
			assertPetriDispatchUsage(t, endpoint, test)
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

func runPetriDispatchUsage(t *testing.T, test petriDispatchUsageCase) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "simple_pipeline"))
	support.WriteAgentConfig(t, dir, "processor", test.workerConfig)
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var openingCustomSession bool
	var customRuntimeIDIssued atomic.Bool
	recordPath := filepath.Join(t.TempDir(), "petri-dispatch-usage.json")
	providerRunner := testutil.NewProviderCommandRunner(test.providerResult)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--record", recordPath},
		Edges: serviceedges.Edges{
			FactoryRuntimeIDGenerator: func() string {
				if openingCustomSession && customRuntimeIDIssued.CompareAndSwap(false, true) {
					return sessionID
				}
				return uuid.NewString()
			},
			FactorySessionIDGenerator:                func() string { return sessionID },
			FactorySessionRuntimeInstanceIDGenerator: uuid.NewString,
			ProviderCommandRunner:                    providerRunner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	openingCustomSession = true
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
	support.CloseFactorySessionAt(t, server.URL(), sessionID)
	return strings.TrimSuffix(server.URL(), "/") + "/factory-sessions/" + sessionID + "/dispatches"
}

func assertPetriDispatchUsage(t *testing.T, endpoint string, test petriDispatchUsageCase) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var (
		body   []byte
		listed factoryapi.ListFactorySessionDispatchesResponse
	)
	for {
		response, err := http.Get(endpoint)
		if err != nil {
			t.Fatalf("GET %s: %v", endpoint, err)
		}
		body, err = io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatalf("read GET %s response: %v", endpoint, err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200: %s", endpoint, response.StatusCode, body)
		}
		listed = factoryapi.ListFactorySessionDispatchesResponse{}
		if err := json.Unmarshal(body, &listed); err != nil {
			t.Fatalf("decode GET %s response: %v", endpoint, err)
		}
		if len(listed.Dispatches) > 0 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("GET %s dispatches remained empty after waiting for recording projection", endpoint)
		case <-ticker.C:
		}
	}
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
