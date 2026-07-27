//go:build functionallong

package claude

import (
	"encoding/json"
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

// TestClaudeGoldenFullStreamTextSuccess replays a sanitized Claude full-stream
// transcript through the customer process boundary and proves public Factory
// response events, Provider Session metadata, and the terminal invocation
// result expose streaming text deltas and a truthful final snapshot.
//
//golden: docs/temp/functional/provider-sessions/claude/full-stream-text-success/manifest.json
func TestClaudeGoldenFullStreamTextSuccess(t *testing.T) {
	support.SkipLongFunctional(t, "slow Claude golden full-stream replay")

	loaded := loadClaudeGoldenCase(t, "full-stream-text-success")
	observed := replayClaudeGoldenCase(t, loaded)
	assertProviderSessionGoldensMatch(t, loaded, observed)
}

// TestClaudeGoldenToolLifecycleAndSessionIdentity replays a sanitized Claude
// tool/session transcript through the customer process boundary and proves
// public Factory response events, Provider Session metadata, and the terminal
// invocation result expose tool start/completion lifecycle plus the stable
// Provider Session identity supplied by the golden transcript.
//
//golden: docs/temp/functional/provider-sessions/claude/tool-lifecycle-session-identity/manifest.json
func TestClaudeGoldenToolLifecycleAndSessionIdentity(t *testing.T) {
	support.SkipLongFunctional(t, "slow Claude golden tool lifecycle replay")

	loaded := loadClaudeGoldenCase(t, "tool-lifecycle-session-identity")
	observed := replayClaudeGoldenCase(t, loaded)
	assertProviderSessionGoldensMatch(t, loaded, observed)
}

func replayClaudeGoldenCase(t *testing.T, loaded support.ProviderSessionCase) support.ProviderSessionObservedGoldens {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderClaude, loaded.Process.Model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"claude golden full-stream text success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("Claude command runner calls = %d, want 1", runner.CallCount())
	}

	return observeProviderSessionGoldens(t, events, responseEvents)
}

func observeProviderSessionGoldens(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) support.ProviderSessionObservedGoldens {
	t.Helper()

	inferenceResponse, ok := successfulInferenceResponsePayload(t, events)
	if !ok {
		t.Fatal("missing successful INFERENCE_RESPONSE factory event")
	}

	providerSessionRaw, err := marshalProviderSessionGoldenJSON(inferenceResponse.ProviderSession)
	if err != nil {
		t.Fatalf("marshal observed provider session: %v", err)
	}

	responseEventRecords := make([]json.RawMessage, 0, len(responseEvents))
	for index, event := range responseEvents {
		record, err := marshalProviderSessionGoldenJSON(event)
		if err != nil {
			t.Fatalf("marshal observed response event[%d]: %v", index, err)
		}
		responseEventRecords = append(responseEventRecords, record)
	}

	invocationResult, err := marshalProviderSessionGoldenJSON(claudeInvocationResultGolden(inferenceResponse))
	if err != nil {
		t.Fatalf("marshal observed invocation result: %v", err)
	}

	return support.ProviderSessionObservedGoldens{
		ProviderSession:   providerSessionRaw,
		ResponseEvents:   responseEventRecords,
		InvocationResult: invocationResult,
	}
}
