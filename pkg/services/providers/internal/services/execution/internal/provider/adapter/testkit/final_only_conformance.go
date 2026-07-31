package testkit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	finalOnlyRunID      = "run-final-only-conformance"
	finalOnlyDispatchID = "dispatch-final-only-conformance"
)

// FinalOnlyFixture supplies completed command results and the neutral facts a
// provider without a native semantic stream must produce from them.
type FinalOnlyFixture struct {
	NewAdapter          func() adapter.Adapter
	Request             workerexecution.ProviderInferenceRequest
	Success             workers.CommandResult
	Failures            []FinalOnlyFailureCase
	Expected            FinalOnlyExpected
	ForbiddenDiagnostic []string
}

// FinalOnlyFailureCase describes one completed output that must normalize to
// an explicit safe failure instead of an empty successful transcript.
type FinalOnlyFailureCase struct {
	Name                    string
	Result                  workers.CommandResult
	ExpectedProviderSession *workerexecution.ProviderSessionMetadata
}

// FinalOnlyExpected identifies the authoritative result expected after a
// successful final-only command.
type FinalOnlyExpected struct {
	Content         string
	ProviderSession *workerexecution.ProviderSessionMetadata
}

// RunFinalOnly runs the shared conformance contract for an adapter that emits
// semantic output only after its command has completed.
func RunFinalOnly(t *testing.T, fixture FinalOnlyFixture) {
	t.Helper()
	requireFinalOnlyFixture(t, fixture)

	t.Run("capabilities", func(t *testing.T) {
		assertFinalOnlyContract(t, fixture.NewAdapter(), fixture.Request)
	})
	t.Run("native output does not fabricate streaming activity", func(t *testing.T) {
		assertNoStreamDrafts(t, fixture)
	})
	t.Run("completed output becomes one authoritative final response", func(t *testing.T) {
		assertFinalOnlySuccess(t, fixture)
	})
	for _, failureCase := range fixture.Failures {
		failureCase := failureCase
		t.Run(failureCase.Name, func(t *testing.T) {
			assertFinalOnlyFailure(t, fixture, failureCase)
		})
	}
}

func assertFinalOnlyContract(t *testing.T, providerAdapter adapter.Adapter, request workerexecution.ProviderInferenceRequest) {
	t.Helper()
	command, err := providerAdapter.BuildCommand(context.Background(), adapter.CommandContext{Request: request})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	if strings.TrimSpace(command.Request.Command) == "" {
		t.Fatal("BuildCommand() returned an empty command")
	}
	result, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{Request: request})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	got := result.Capabilities
	if got.NativeStreaming || got.MessageDeltas || !got.MessageSnapshots || got.ReasoningSummaries || got.ToolLifecycle || got.ToolOutputDeltas || got.StableItemIDs || !got.FinalOnly {
		t.Fatalf("final-only capabilities = %#v", got)
	}
}

func assertNoStreamDrafts(t *testing.T, fixture FinalOnlyFixture) {
	t.Helper()
	observations := []adapter.Observation{
		{Stream: adapter.OutputStreamStdout, Chunk: fixture.Success.Stdout},
		{Stream: adapter.OutputStreamStderr, Chunk: fixture.Success.Stderr},
	}
	drafts, diagnostics := decode(t, fixture.NewAdapter(), observations, adapter.FlushReasonCompleted)
	if len(drafts) != 0 {
		t.Fatalf("final-only decoder fabricated stream drafts: %#v", drafts)
	}
	assertSafeDiagnostics(t, diagnostics, fixture.ForbiddenDiagnostic)
}

func assertFinalOnlySuccess(t *testing.T, fixture FinalOnlyFixture) {
	t.Helper()
	result, err := fixture.NewAdapter().ParseFinal(context.Background(), finalOnlyParseContext(fixture.Success))
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != fixture.Expected.Content {
		t.Fatalf("final content = %q, want %q", result.Response.Content, fixture.Expected.Content)
	}
	if !sameProviderSession(result.Response.ProviderSession, fixture.Expected.ProviderSession) {
		t.Fatalf("provider session = %#v, want %#v", result.Response.ProviderSession, fixture.Expected.ProviderSession)
	}
	assertFinalOnlyDrafts(t, result.Drafts, fixture.Expected)
}

func assertFinalOnlyDrafts(t *testing.T, drafts []responseevents.Draft, expected FinalOnlyExpected) {
	t.Helper()
	if len(drafts) != 3 {
		t.Fatalf("final drafts = %#v, want run start, message completion, and run completion", drafts)
	}
	want := []struct {
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{responseevents.KindRun, responseevents.PhaseStarted},
		{responseevents.KindMessage, responseevents.PhaseCompleted},
		{responseevents.KindRun, responseevents.PhaseCompleted},
	}
	for index, draft := range drafts {
		if err := responseevents.ValidateDraft(draft); err != nil {
			t.Fatalf("draft[%d] is not canonical: %v", index, err)
		}
		if draft.Kind != want[index].kind || draft.Phase != want[index].phase {
			t.Fatalf("draft[%d] = %s/%s, want %s/%s", index, draft.Kind, draft.Phase, want[index].kind, want[index].phase)
		}
		if draft.RunID != finalOnlyRunID || draft.DispatchID != finalOnlyDispatchID || draft.ItemID != "" {
			t.Fatalf("draft[%d] fabricated or lost correlation: %#v", index, draft)
		}
	}
	message := drafts[1]
	wantProviderRef := ""
	if expected.ProviderSession != nil {
		wantProviderRef = expected.ProviderSession.ID
	}
	if message.ProviderSessionRef != wantProviderRef || messageText(t, message) != expected.Content {
		t.Fatalf("authoritative message draft = %#v", message)
	}
	if message.Provenance.Delivery != responseevents.DeliveryNativeFinal || message.Provenance.Representation != responseevents.RepresentationSnapshot || message.Provenance.Fidelity != responseevents.FidelityFinalOnly {
		t.Fatalf("message provenance = %#v", message.Provenance)
	}
}

func assertFinalOnlyFailure(t *testing.T, fixture FinalOnlyFixture, failureCase FinalOnlyFailureCase) {
	t.Helper()
	providerAdapter := fixture.NewAdapter()
	_, parseErr := providerAdapter.ParseFinal(context.Background(), finalOnlyParseContext(failureCase.Result))
	if parseErr == nil {
		t.Fatal("ParseFinal() error = nil, want explicit normalized failure")
	}
	assertSafeText(t, "parse error", parseErr.Error(), fixture.ForbiddenDiagnostic)
	result := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: failureCase.Result,
		ParseError:    parseErr,
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if result.Failure == nil || result.Failure.Family != workerexecution.WorkFailureFamilyTerminal || result.Failure.Type == "" || result.Failure.Retry.Retryable {
		t.Fatalf("ClassifyFailure() = %#v", result)
	}
	assertSafeText(t, "failure message", result.Failure.Message, fixture.ForbiddenDiagnostic)
	if !sameProviderSession(result.Failure.ProviderSession, failureCase.ExpectedProviderSession) {
		t.Fatalf("failure provider session = %#v, want %#v", result.Failure.ProviderSession, failureCase.ExpectedProviderSession)
	}
}

func finalOnlyParseContext(result workers.CommandResult) adapter.FinalParseContext {
	return adapter.FinalParseContext{
		CommandResult: result,
		FlushReason:   adapter.FlushReasonCompleted,
		RunID:         finalOnlyRunID,
		DispatchID:    finalOnlyDispatchID,
	}
}

func sameProviderSession(got, want *workerexecution.ProviderSessionMetadata) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func requireFinalOnlyFixture(t *testing.T, fixture FinalOnlyFixture) {
	t.Helper()
	requireNoError(t, validateFinalOnlyFixture(fixture))
}

func validateFinalOnlyFixture(fixture FinalOnlyFixture) error {
	if fixture.NewAdapter == nil || strings.TrimSpace(fixture.Expected.Content) == "" {
		return fmt.Errorf("NewAdapter and non-empty expected content are required")
	}
	if len(fixture.Failures) < 2 {
		return fmt.Errorf("empty and malformed final-output failure fixtures are required")
	}
	return nil
}
