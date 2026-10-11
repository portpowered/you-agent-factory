package acp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Isolation: isolated-with-reason - pinned wire transcript; sharing would
// erase the exact initialize/session/prompt/update ordering witness.
func TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP request"}`))
	writeACPWorker(t, dir, "cursor")
	writeGoldenSentinelWorkstation(t, dir)
	fixture := goldenACPFixture("success")

	var starts atomic.Int32
	_, listed, factoryEvents, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: goldenACPCommandFactory(&starts, fixture),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		details, _ := json.MarshalIndent(struct {
			Work   factoryapi.ListWorkResponse `json:"work"`
			Events []factoryapi.FactoryEvent   `json:"events"`
		}{listed, factoryEvents}, "", "  ")
		t.Fatalf("completed work = %d, want 1; observations=%s", got, details)
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
	assertGoldenProviderSession(t, factoryEvents)
	assertGoldenResponseStream(t, responseEvents)
}

func writeGoldenSentinelWorkstation(t *testing.T, dir string) {
	t.Helper()
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  YOU_ACP_GOLDEN_SENTINEL: preserved
---

Test workstation.
`)
}

func assertGoldenProviderSession(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := event.Payload.AsModelResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.ProviderSession != nil && payload.ProviderSession.Provider != nil && payload.ProviderSession.Id != nil &&
			*payload.ProviderSession.Provider == "cursor" && *payload.ProviderSession.Id == "sess_abc123def456" {
			return
		}
	}
	t.Fatal("Factory events omitted the golden ACP Provider Session")
}

type stableResponseEvent struct {
	Kind       string `json:"kind"`
	Phase      string `json:"phase"`
	Provider   string `json:"provider"`
	NativeType string `json:"nativeType"`
	ItemID     string `json:"itemId"`
}

// assertGoldenResponseStream pins the exact response-event order produced by
// the scripted ACP peer, including REASONING and title-bearing SESSION
// metadata before the terminal MESSAGE and RUN records.
func assertGoldenResponseStream(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	var lines []string
	var completedMessages int
	for _, event := range events {
		if event.Provenance.Provider != "cursor" && event.Provenance.Provider != "agent-run" {
			continue
		}
		itemID := ""
		if event.ItemId != nil {
			itemID = *event.ItemId
		}
		if event.Kind == "MESSAGE" && event.Phase == "COMPLETED" {
			completedMessages++
			assertGoldenFinalMessage(t, event)
			// The fallback identity is dispatch-scoped, so pin its relation to
			// this execution before normalizing the generated ID for the golden.
			if event.DispatchId == nil || itemID != *event.DispatchId+"-final-message" {
				t.Fatalf("ACP final message identity = %q, dispatch = %v", itemID, event.DispatchId)
			}
			itemID = "dispatch-final-message"
		}
		stable := stableResponseEvent{
			Kind: string(event.Kind), Phase: string(event.Phase), Provider: event.Provenance.Provider,
			NativeType: event.Provenance.NativeEventType, ItemID: itemID,
		}
		encoded, err := json.Marshal(stable)
		if err != nil {
			t.Fatalf("marshal stable ACP response event: %v", err)
		}
		lines = append(lines, string(encoded))
	}
	if completedMessages != 1 {
		t.Fatalf("ACP completed assistant messages = %d, want exactly one", completedMessages)
	}
	got := strings.Join(lines, "\n") + "\n"
	want, err := os.ReadFile(filepath.Join("testdata", "json_golden", "expected", "response_stream.ndjson"))
	if err != nil {
		t.Fatalf("read ACP response stream golden: %v\nactual:\n%s", err, got)
	}
	if got != string(want) {
		t.Fatalf("ACP response stream differs from golden\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func assertGoldenFinalMessage(t *testing.T, event factoryapi.FactoryResponseEvent) {
	t.Helper()
	// This ACP fixture returns accumulated text without a native completed
	// message. Workers publishes one truthful fallback; filtering on cursor
	// alone would discard it and incorrectly report missing terminal output.
	if event.Provenance.Provider != "agent-run" || event.Provenance.NativeEventType != "agent_final_response" ||
		event.Provenance.Delivery != "SYNTHESIZED" || event.Provenance.Fidelity != "FINAL_ONLY" || event.Provenance.Representation != "SNAPSHOT" {
		t.Fatalf("ACP final fallback provenance = %+v", event.Provenance)
	}
	payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
	if err != nil || payload.Role != "assistant" || len(payload.ContentBlocks) != 1 {
		t.Fatalf("ACP final message payload = %+v, error = %v", payload, err)
	}
	text, err := payload.ContentBlocks[0].AsFactoryResponseEventTextContentBlock()
	if err != nil || text.Text != "The capital of France is Paris. golden ACP COMPLETE" {
		t.Fatalf("ACP final text = %+v, error = %v", text, err)
	}
}
