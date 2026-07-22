package providerparity

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

func TestCatalog_CoversAllFidelityClasses(t *testing.T) {
	t.Parallel()

	want := map[FidelityClass]bool{
		FidelityFullStream:    false,
		FidelityPartialStream: false,
		FidelitySnapshotOnly:  false,
		FidelityFinalOnly:     false,
	}
	var toolLifecycle, agyFinalOnly bool
	for _, fixture := range Catalog() {
		want[fixture.FidelityClass] = true
		if fixture.ToolLifecycle {
			toolLifecycle = true
		}
		if fixture.AgyFinalOnly {
			agyFinalOnly = true
		}
		if fixture.ID == "" || fixture.TranscriptFile == "" || fixture.WantContent == "" {
			t.Fatalf("incomplete fixture: %#v", fixture)
		}
	}
	for class, covered := range want {
		if !covered {
			t.Fatalf("catalog missing fidelity class %q", class)
		}
	}
	if !toolLifecycle {
		t.Fatal("catalog missing structured tool lifecycle fixture")
	}
	if !agyFinalOnly {
		t.Fatal("catalog missing Agy final-only fixture")
	}
}

func TestSanitizedTranscripts_ExcludeForbiddenTokens(t *testing.T) {
	t.Parallel()

	for _, fixture := range Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			transcript, err := ReadTranscript(fixture.TranscriptFile)
			if err != nil {
				t.Fatalf("ReadTranscript() error = %v", err)
			}
			if err := ValidateSanitized(transcript); err != nil {
				t.Fatalf("ValidateSanitized() error = %v", err)
			}
		})
	}
}

func TestHarness_ReachesTerminalOutcome(t *testing.T) {
	t.Parallel()

	for _, fixture := range Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			result, err := RunTerminal(context.Background(), fixture)
			if err != nil {
				t.Fatalf("RunTerminal() error = %v", err)
			}
			if result.Outcome != adapter.CommandOutcomeCompleted {
				t.Fatalf("outcome = %q, want %q", result.Outcome, adapter.CommandOutcomeCompleted)
			}
			if result.Response.Content != fixture.WantContent {
				t.Fatalf("content = %q, want %q", result.Response.Content, fixture.WantContent)
			}
			if result.Capabilities != fixture.WantCapabilities {
				t.Fatalf("capabilities = %#v, want %#v", result.Capabilities, fixture.WantCapabilities)
			}
			if fixture.ToolLifecycle && !hasToolLifecycleDrafts(result.Drafts) {
				t.Fatalf("drafts = %#v, want observable tool lifecycle events", result.Drafts)
			}
			if fixture.FidelityClass == FidelityFinalOnly {
				assertFinalOnlyDrafts(t, result.Drafts)
			}
		})
	}
}

func hasToolLifecycleDrafts(drafts []factorysessions.ResponseEventDraft) bool {
	var started, completed bool
	for _, draft := range drafts {
		if draft.Kind != factorysessions.ResponseEventKindTool {
			continue
		}
		switch draft.Phase {
		case factorysessions.ResponseEventPhaseStarted:
			started = true
		case factorysessions.ResponseEventPhaseCompleted:
			completed = true
		}
	}
	return started && completed
}

func assertFinalOnlyDrafts(t *testing.T, drafts []factorysessions.ResponseEventDraft) {
	t.Helper()
	var message *factorysessions.ResponseEventDraft
	for index := range drafts {
		draft := drafts[index]
		if draft.Kind == factorysessions.ResponseEventKindMessage && draft.Phase == factorysessions.ResponseEventPhaseCompleted {
			message = &draft
			break
		}
	}
	if message == nil {
		t.Fatalf("drafts = %#v, want final-only completed message", drafts)
	}
	if message.Provenance.Fidelity != factorysessions.ResponseEventFidelityFinalOnly {
		t.Fatalf("message fidelity = %q, want %q", message.Provenance.Fidelity, factorysessions.ResponseEventFidelityFinalOnly)
	}
}
