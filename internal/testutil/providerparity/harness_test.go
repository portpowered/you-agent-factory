package providerparity_test

import (
	"context"
	"testing"

	parityfixtures "github.com/portpowered/infinite-you/internal/testutil/providerparity"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

func TestCatalog_CoversAllFidelityClasses(t *testing.T) {
	t.Parallel()

	want := map[parityfixtures.FidelityClass]bool{
		parityfixtures.FidelityFullStream:    false,
		parityfixtures.FidelityPartialStream: false,
		parityfixtures.FidelitySnapshotOnly:  false,
		parityfixtures.FidelityFinalOnly:     false,
	}
	var toolLifecycle, agyFinalOnly bool
	for _, fixture := range parityfixtures.Catalog() {
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

	for _, fixture := range parityfixtures.Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			transcript, err := parityfixtures.ReadTranscript(fixture.TranscriptFile)
			if err != nil {
				t.Fatalf("ReadTranscript() error = %v", err)
			}
			if err := parityfixtures.ValidateSanitized(transcript); err != nil {
				t.Fatalf("ValidateSanitized() error = %v", err)
			}
		})
	}
}

func TestHarness_ReachesTerminalOutcome(t *testing.T) {
	t.Parallel()

	for _, fixture := range parityfixtures.Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			result, err := parityfixtures.RunTerminal(context.Background(), fixture)
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
			if fixture.FidelityClass == parityfixtures.FidelityFinalOnly {
				assertFinalOnlyDrafts(t, result.Drafts)
			}
		})
	}
}

func hasToolLifecycleDrafts(drafts []responseevents.Draft) bool {
	var started, completed bool
	for _, draft := range drafts {
		if draft.Kind != responseevents.KindTool {
			continue
		}
		switch draft.Phase {
		case responseevents.PhaseStarted:
			started = true
		case responseevents.PhaseCompleted:
			completed = true
		}
	}
	return started && completed
}

func assertFinalOnlyDrafts(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	var message *responseevents.Draft
	for index := range drafts {
		draft := drafts[index]
		if draft.Kind == responseevents.KindMessage && draft.Phase == responseevents.PhaseCompleted {
			message = &draft
			break
		}
	}
	if message == nil {
		t.Fatalf("drafts = %#v, want final-only completed message", drafts)
	}
	if message.Provenance.Fidelity != responseevents.FidelityFinalOnly {
		t.Fatalf("message fidelity = %q, want %q", message.Provenance.Fidelity, responseevents.FidelityFinalOnly)
	}
}
