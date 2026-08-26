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

func TestYouRunSendsInputWorkContentAsACPText(t *testing.T) {
	t.Parallel()
	const sentinel = "ACP_INPUT_WORK_CONTENT_9f31"
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"`+sentinel+`"}`))
	writeACPWorker(t, dir, "cursor-acp")

	var starts atomic.Int32
	fixture := functionalACPFixture("content")
	fixture.ContentSentinel = sentinel
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts, fixture),
		ProvidersExecutableLocator:    availableExecutableLocator{},
	}, 20*time.Second)
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; events=%#v", got, events)
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
}

func TestYouRunUsesPinnedACPWireGoldensAndProjectsTerminalOutput(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"golden ACP request"}`))
	writeACPWorker(t, dir, "cursor-acp")
	t.Setenv("YOU_ACP_GOLDEN_SENTINEL", "preserved")

	var starts atomic.Int32
	_, listed, factoryEvents, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(t, dir, serviceedges.Edges{
		PlatformProcessCommandFactory: goldenACPCommandFactory(&starts, "success"),
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
			*payload.ProviderSession.Provider == "cursor-acp" && *payload.ProviderSession.Id == "sess_abc123def456" {
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
	for _, event := range events {
		if event.Provenance.Provider != "cursor-acp" {
			continue
		}
		itemID := ""
		if event.ItemId != nil {
			itemID = *event.ItemId
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
	got := strings.Join(lines, "\n") + "\n"
	want, err := os.ReadFile(filepath.Join("testdata", "json_golden", "expected", "response_stream.ndjson"))
	if err != nil {
		t.Fatalf("read ACP response stream golden: %v\nactual:\n%s", err, got)
	}
	if got != string(want) {
		t.Fatalf("ACP response stream differs from golden\nwant:\n%s\ngot:\n%s", want, got)
	}
}
