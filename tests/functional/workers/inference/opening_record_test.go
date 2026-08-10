package inference_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWSRFT003ProviderNeutralLifecycleWorksWithoutProviderSession replays the
// sanitized Antigravity final-only fixture through the customer process
// boundary. It proves the provider-neutral output has honest final-only
// fidelity and that neither the inference event nor response stream invents a
// Provider Session reference that the provider did not emit.
//
// WSR-FT-003: provider-independent opening/terminal history and no fabricated
// Provider Session reference.
// golden: tests/functional/internal/support/testdata/provider-sessions/agy/final-only-success/manifest.json
func TestWSRFT003ProviderNeutralLifecycleWorksWithoutProviderSession(t *testing.T) {
	loaded := loadOpeningRecordFixture(t, "agy", "final-only-success")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, loaded.Process.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"WSR-FT-003 provider-neutral lifecycle"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})
	_, listed, factoryEvents, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want 1", runner.CallCount())
	}
	assertAgyOpeningRecordProviderNeutrality(t, factoryEvents)
	assertAgyFinalOnlyOpeningRecord(t, responseEvents)
}

func loadOpeningRecordFixture(
	t *testing.T,
	fixtureProvider string,
	caseName string,
) support.ProviderSessionCase {
	t.Helper()
	caseDir := filepath.Join(
		testutil.MustRepoRoot(t),
		filepath.FromSlash(support.ProviderSessionFixturePath(fixtureProvider, caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q/%q): %v", fixtureProvider, caseName, err)
	}
	return loaded
}

func assertAgyOpeningRecordProviderNeutrality(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse && event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		observation, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode Antigravity inference response: %v", err)
		}
		if observation.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		if observation.ProviderSession != nil &&
			observation.ProviderSession.Id != nil &&
			strings.TrimSpace(*observation.ProviderSession.Id) != "" {
			t.Fatalf("Antigravity final-only response fabricated Provider Session id: %#v", observation.ProviderSession)
		}
		if observation.Response == nil || !strings.Contains(*observation.Response, "COMPLETE") {
			t.Fatalf("Antigravity response = %#v, want successful COMPLETE-bearing final output", observation.Response)
		}
		return
	}
	t.Fatalf("Factory Event history omitted successful Antigravity inference response: %#v", events)
}

func assertAgyFinalOnlyOpeningRecord(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("response-event history is empty")
	}
	completedMessage := false
	for _, event := range events {
		if event.ProviderSessionRef != nil && strings.TrimSpace(*event.ProviderSessionRef) != "" {
			t.Fatalf("Antigravity response event fabricated Provider Session reference: %#v", event)
		}
		if event.Kind == factoryapi.FactoryResponseEventKindMessage {
			switch event.Phase {
			case factoryapi.FactoryResponseEventPhaseDelta:
				t.Fatalf("Antigravity final-only response fabricated MESSAGE/DELTA: %#v", event)
			case factoryapi.FactoryResponseEventPhaseCompleted:
				if event.Provenance.Provider != string(modelprovider.ProviderAntigravity) ||
					event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot ||
					event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityLossless {
					t.Fatalf("Antigravity final message provenance = %#v, want provider-attributed normalized snapshot", event.Provenance)
				}
				payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
				if err != nil {
					t.Fatalf("decode Antigravity final message: %v", err)
				}
				encoded, err := json.Marshal(payload)
				if err != nil || !strings.Contains(string(encoded), "COMPLETE") {
					t.Fatalf("Antigravity final message = %#v, want COMPLETE-bearing snapshot", payload)
				}
				completedMessage = true
			}
		}
	}
	if !completedMessage {
		t.Fatalf("Antigravity response stream omitted final authoritative message: %#v", events)
	}
}
