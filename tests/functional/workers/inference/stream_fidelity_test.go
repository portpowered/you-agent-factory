package inference_test

import (
	"fmt"
	"os"
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

const (
	claudeFullStreamGoldenCase      = "full-stream-text-success"
	codexPartialStreamGoldenCase    = "success"
	openCodeSnapshotOnlyGoldenCase  = "structured-snapshot-success"
)

// TestProviderFullStreamClaimsDeltasAndSnapshotsTruthfully replays a sanitized
// full-stream provider transcript through root.BuildProcess and proves published
// Factory response events claim native streaming with truthful message deltas and
// a matching completed snapshot, without final-only fidelity or synthesized
// message-delta fabrication.
func TestProviderFullStreamClaimsDeltasAndSnapshotsTruthfully(t *testing.T) {
	loaded := loadStreamFidelityGoldenCase(t, modelprovider.ProviderClaude, claudeFullStreamGoldenCase)
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFullStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFullStream,
		)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, loaded.Process.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider full stream fidelity"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one provider-process edge", runner.CallCount())
	}

	assertFullStreamPublicResponseEvents(t, responseEvents, "Parity hello world COMPLETE")
}

// TestProviderPartialStreamDoesNotFabricateMissingDeltas replays a sanitized
// partial-stream provider transcript through root.BuildProcess and proves
// published Factory response events expose native streaming with completed
// message snapshots only—no fabricated message deltas and no final-only fidelity
// on those snapshots.
func TestProviderPartialStreamDoesNotFabricateMissingDeltas(t *testing.T) {
	loaded := loadStreamFidelityGoldenCase(t, modelprovider.ProviderCodex, codexPartialStreamGoldenCase)
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider partial stream fidelity"}`))

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
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one provider-process edge", runner.CallCount())
	}

	assertPartialStreamPublicResponseEvents(t, responseEvents, factoryEvents, "Codex fixture answer COMPLETE")
}

// TestProviderSnapshotOnlyEmitsCompletedSnapshotsOnly replays a sanitized
// snapshot-only provider transcript through root.BuildProcess and proves
// published Factory response events expose native streaming with completed
// message snapshots only—zero message deltas and no final-only fidelity claims.
func TestProviderSnapshotOnlyEmitsCompletedSnapshotsOnly(t *testing.T) {
	loaded := loadStreamFidelityGoldenCase(t, modelprovider.ProviderOpenCode, openCodeSnapshotOnlyGoldenCase)
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelitySnapshotOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelitySnapshotOnly,
		)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderOpenCode, loaded.Process.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"provider snapshot-only fidelity"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	executablePath := writeStreamFidelityOpenCodeExecutable(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("1.2.3\n")},
		platformprocess.CommandResult{ExitCode: 0},
		platformprocess.CommandResult{
			Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
			Stderr:   []byte(loaded.Stderr),
			ExitCode: exitCode,
		},
	)

	_, listed, _, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{
			ProviderCommandRunner:    runner,
			WorkersExecutableLocator: streamFidelityOpenCodeExecutableLocator{path: executablePath},
			WorkersResolveSymlinks:   streamFidelityIdentityResolveSymlinks,
		},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 3 {
		t.Fatalf("provider command runner calls = %d, want discovery probes plus one invocation", runner.CallCount())
	}

	assertSnapshotOnlyPublicResponseEvents(t, responseEvents, "Hello world COMPLETE")
}

func loadStreamFidelityGoldenCase(
	t *testing.T,
	provider modelprovider.Provider,
	caseName string,
) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath(string(provider), caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", caseName, err)
	}
	return loaded
}

func assertFullStreamPublicResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	wantCompletedText string,
) {
	t.Helper()

	var (
		deltaCount      int
		deltaText       strings.Builder
		completedText   string
		completedSeen   bool
	)

	for _, event := range events {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}

		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseDelta:
			if event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityFinalOnly {
				t.Fatalf("full-stream replay claimed final-only fidelity on message delta: %#v", event)
			}
			if event.Provenance.Delivery != factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream {
				t.Fatalf(
					"full-stream message delta delivery = %q, want %q: %#v",
					event.Provenance.Delivery,
					factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream,
					event,
				)
			}
			if event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationDelta {
				t.Fatalf(
					"full-stream message delta representation = %q, want %q: %#v",
					event.Provenance.Representation,
					factoryapi.FactoryResponseEventProvenanceRepresentationDelta,
					event,
				)
			}

			payload, err := event.Payload.AsFactoryResponseEventMessageDeltaPayload()
			if err != nil {
				t.Fatalf("decode message delta response event: %v", err)
			}
			if payload.TextDelta == nil || strings.TrimSpace(*payload.TextDelta) == "" {
				t.Fatalf("full-stream message delta missing text: %#v", event)
			}
			deltaCount++
			deltaText.WriteString(*payload.TextDelta)

		case factoryapi.FactoryResponseEventPhaseCompleted:
			if event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityFinalOnly {
				t.Fatalf("full-stream replay claimed final-only fidelity on completed message: %#v", event)
			}
			if event.Provenance.Delivery != factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream {
				t.Fatalf(
					"full-stream completed message delivery = %q, want %q: %#v",
					event.Provenance.Delivery,
					factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream,
					event,
				)
			}
			if event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot {
				t.Fatalf(
					"full-stream completed message representation = %q, want %q: %#v",
					event.Provenance.Representation,
					factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot,
					event,
				)
			}

			payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				t.Fatalf("decode completed message response event: %v", err)
			}
			completedText = streamFidelityMessageText(payload)
			if completedText == "" {
				t.Fatalf("full-stream completed message missing text: %#v", event)
			}
			completedSeen = true
		}
	}

	if deltaCount == 0 {
		t.Fatal("full-stream replay missing native message deltas")
	}
	if !completedSeen {
		t.Fatal("full-stream replay missing completed message snapshot")
	}
	if got := strings.TrimSpace(deltaText.String()); got != wantCompletedText {
		t.Fatalf("concatenated message deltas = %q, want %q", got, wantCompletedText)
	}
	if completedText != wantCompletedText {
		t.Fatalf("completed message snapshot = %q, want %q", completedText, wantCompletedText)
	}
}

func assertPartialStreamPublicResponseEvents(
	t *testing.T,
	responseEvents []factoryapi.FactoryResponseEvent,
	factoryEvents []factoryapi.FactoryEvent,
	wantCompletedText string,
) {
	t.Helper()

	var (
		snapshotCount   int
		terminalMatched bool
	)

	for _, event := range responseEvents {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}

		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseDelta:
			t.Fatalf("partial-stream replay fabricated message delta: %#v", event)

		case factoryapi.FactoryResponseEventPhaseCompleted:
			if event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityFinalOnly {
				t.Fatalf("partial-stream replay claimed final-only fidelity on completed message: %#v", event)
			}
			if event.Provenance.Delivery != factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream {
				t.Fatalf(
					"partial-stream completed message delivery = %q, want %q: %#v",
					event.Provenance.Delivery,
					factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream,
					event,
				)
			}
			if event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot {
				t.Fatalf(
					"partial-stream completed message representation = %q, want %q: %#v",
					event.Provenance.Representation,
					factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot,
					event,
				)
			}

			payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				t.Fatalf("decode completed message response event: %v", err)
			}
			text := streamFidelityMessageText(payload)
			if text == "" {
				t.Fatalf("partial-stream completed message missing text: %#v", event)
			}
			snapshotCount++
			if text == wantCompletedText {
				terminalMatched = true
			}
		}
	}

	if snapshotCount == 0 {
		terminalMatched = streamFidelityInferenceResponseContains(t, factoryEvents, wantCompletedText)
	}
	if !terminalMatched {
		t.Fatalf("partial-stream replay missing terminal message %q", wantCompletedText)
	}
}

func assertSnapshotOnlyPublicResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	wantCompletedText string,
) {
	t.Helper()

	var (
		snapshotCount int
		terminalSeen  bool
	)

	for _, event := range events {
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			continue
		}

		switch event.Phase {
		case factoryapi.FactoryResponseEventPhaseDelta:
			t.Fatalf("snapshot-only replay fabricated message delta: %#v", event)

		case factoryapi.FactoryResponseEventPhaseCompleted:
			if event.Provenance.Fidelity == factoryapi.FactoryResponseEventProvenanceFidelityFinalOnly {
				t.Fatalf("snapshot-only replay claimed final-only fidelity on completed message: %#v", event)
			}
			if event.Provenance.Delivery != factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream {
				t.Fatalf(
					"snapshot-only completed message delivery = %q, want %q: %#v",
					event.Provenance.Delivery,
					factoryapi.FactoryResponseEventProvenanceDeliveryNativeStream,
					event,
				)
			}
			if event.Provenance.Representation != factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot {
				t.Fatalf(
					"snapshot-only completed message representation = %q, want %q: %#v",
					event.Provenance.Representation,
					factoryapi.FactoryResponseEventProvenanceRepresentationSnapshot,
					event,
				)
			}

			payload, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				t.Fatalf("decode completed message response event: %v", err)
			}
			text := streamFidelityMessageText(payload)
			if text == "" {
				t.Fatalf("snapshot-only completed message missing text: %#v", event)
			}
			snapshotCount++
			if text == wantCompletedText {
				terminalSeen = true
			}
		}
	}

	if snapshotCount == 0 {
		t.Fatal("snapshot-only replay missing completed message snapshots")
	}
	if !terminalSeen {
		t.Fatalf("snapshot-only replay missing terminal completed message %q", wantCompletedText)
	}
}

type streamFidelityOpenCodeExecutableLocator struct {
	path string
}

func (l streamFidelityOpenCodeExecutableLocator) LookPath(file string) (string, error) {
	if file == "opencode" {
		return l.path, nil
	}
	return "", fmt.Errorf("executable %q not found", file)
}

func streamFidelityIdentityResolveSymlinks(path string) (string, error) {
	return path, nil
}

func writeStreamFidelityOpenCodeExecutable(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "opencode")
	if err := os.WriteFile(path, []byte("opencode-fixture-executable\n"), 0o755); err != nil {
		t.Fatalf("write opencode fixture executable: %v", err)
	}
	return path
}

func streamFidelityInferenceResponseContains(
	t *testing.T,
	factoryEvents []factoryapi.FactoryEvent,
	wantText string,
) bool {
	t.Helper()

	for _, event := range factoryEvents {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		if payload.Response != nil && strings.Contains(*payload.Response, wantText) {
			return true
		}
	}
	return false
}

func streamFidelityMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range message.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}
