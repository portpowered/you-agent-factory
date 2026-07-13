package testkit

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

// SnapshotStreamFixture describes a native stream whose authoritative message
// is a completed snapshot rather than a sequence of message deltas.
type SnapshotStreamFixture struct {
	NewAdapter          func() adapter.Adapter
	Request             interfaces.ProviderInferenceRequest
	Content             []adapter.Observation
	TerminalFailure     []adapter.Observation
	UnsafeAndRecovering []adapter.Observation
	UnterminatedFinal   []adapter.Observation
	FinalResult         workerprocess.CommandResult
	Expected            SnapshotStreamExpected
	ForbiddenDiagnostic []string
}

type SnapshotStreamExpected struct {
	Capabilities     adapter.Capabilities
	Drafts           []DraftExpectation
	ProviderSession  interfaces.ProviderSessionMetadata
	FinalContent     string
	FailureFamily    interfaces.WorkFailureFamily
	FailureType      interfaces.WorkFailureType
	FailureRetryable bool
}

type DraftExpectation struct {
	Kind        responseevents.Kind
	Phase       responseevents.Phase
	ItemID      string
	ProviderRef string
}

// RunSnapshotStream applies the shared lifecycle, safety, flush, final-result,
// and terminal-reconciliation contract to a snapshot-oriented adapter.
func RunSnapshotStream(t *testing.T, fixture SnapshotStreamFixture) {
	t.Helper()
	if fixture.NewAdapter == nil || len(fixture.Content) == 0 || len(fixture.UnsafeAndRecovering) == 0 || len(fixture.UnterminatedFinal) == 0 {
		t.Fatal("snapshot stream fixture is incomplete")
	}
	t.Run("declared capabilities and command", func(t *testing.T) { verifySnapshotCapabilities(t, fixture) })
	t.Run("ordered canonical snapshots", func(t *testing.T) { verifySnapshotDrafts(t, fixture) })
	t.Run("authoritative final result", func(t *testing.T) { verifySnapshotFinal(t, fixture) })
	t.Run("unsafe input recovers", func(t *testing.T) {
		verifyUnsafeRecovery(t, FullStreamFixture{
			NewAdapter: fixture.NewAdapter, UnsafeAndRecovering: fixture.UnsafeAndRecovering,
			ForbiddenDiagnostic: fixture.ForbiddenDiagnostic,
		})
	})
	t.Run("flush processes final unterminated record", func(t *testing.T) {
		drafts, diagnostics := decode(t, fixture.NewAdapter(), fixture.UnterminatedFinal, adapter.FlushReasonCompleted)
		assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
		message := findDraft(drafts, responseevents.KindMessage, responseevents.PhaseCompleted)
		if message == nil || messageText(t, *message) != fixture.Expected.FinalContent {
			t.Fatalf("unterminated final drafts = %#v", drafts)
		}
	})
	if len(fixture.TerminalFailure) > 0 {
		t.Run("terminal failure reconciliation", func(t *testing.T) { verifySnapshotFailure(t, fixture) })
	}
}

func verifySnapshotCapabilities(t *testing.T, fixture SnapshotStreamFixture) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	command, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{Request: fixture.Request})
	if err != nil || strings.TrimSpace(command.Request.Command) == "" {
		t.Fatalf("BuildCommand() = %#v, %v", command, err)
	}
	result, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{Request: fixture.Request})
	if err != nil || !reflect.DeepEqual(result.Capabilities, fixture.Expected.Capabilities) {
		t.Fatalf("Capabilities() = %#v, %v; want %#v", result.Capabilities, err, fixture.Expected.Capabilities)
	}
}

func verifySnapshotDrafts(t *testing.T, fixture SnapshotStreamFixture) {
	t.Helper()
	drafts, diagnostics := decode(t, fixture.NewAdapter(), fixture.Content, adapter.FlushReasonCompleted)
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
	if len(drafts) != len(fixture.Expected.Drafts) {
		t.Fatalf("draft count = %d, want %d: %#v", len(drafts), len(fixture.Expected.Drafts), drafts)
	}
	for index, expected := range fixture.Expected.Drafts {
		got := drafts[index]
		if got.Kind != expected.Kind || got.Phase != expected.Phase || got.ItemID != expected.ItemID || got.ProviderSessionRef != expected.ProviderRef {
			t.Fatalf("draft[%d] = %#v, want %#v", index, got, expected)
		}
	}
}

func verifySnapshotFinal(t *testing.T, fixture SnapshotStreamFixture) {
	t.Helper()
	result, err := fixture.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{CommandResult: fixture.FinalResult, FlushReason: adapter.FlushReasonCompleted})
	if err != nil || result.Response.Content != fixture.Expected.FinalContent || result.Response.ProviderSession == nil || *result.Response.ProviderSession != fixture.Expected.ProviderSession || len(result.Drafts) != 0 {
		t.Fatalf("ParseFinal() = %#v, %v", result, err)
	}
}

func verifySnapshotFailure(t *testing.T, fixture SnapshotStreamFixture) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	stdout := observationsForStream(fixture.TerminalFailure, adapter.OutputStreamStdout)
	_, parseErr := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}})
	if parseErr == nil {
		t.Fatal("ParseFinal() error = nil, want terminal failure")
	}
	result := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{CommandResult: workerprocess.CommandResult{Stdout: stdout}, ParseError: parseErr})
	failure := result.Failure
	if failure == nil || failure.Family != fixture.Expected.FailureFamily || failure.Type != fixture.Expected.FailureType || failure.Retry.Retryable != fixture.Expected.FailureRetryable {
		t.Fatalf("ClassifyFailure() = %#v", result)
	}
	if failure.ProviderSession == nil || *failure.ProviderSession != fixture.Expected.ProviderSession {
		t.Fatalf("failure provider session = %#v", failure.ProviderSession)
	}
	assertSafeText(t, "failure message", failure.Message, fixture.ForbiddenDiagnostic)
}
