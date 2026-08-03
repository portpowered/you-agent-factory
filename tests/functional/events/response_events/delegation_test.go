package response_events

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFactoryResponseEventsSurviveTheEventsAuthoritativePublishPath proves
// that a real session run through root.BuildProcess and Process.Execute --
// the exact customer entrypoint, with only the provider command boundary
// replaced through edges.Edges -- still produces complete, contiguous,
// uniquely identified public Factory response events end to end.
//
// ResponseStream.Publish (pkg/services/factory_sessions/internal/services/
// response_stream/internal/service/service.go) assigns identity through the
// injected Events root before this session's store ever observes a record:
// Events.Append is the authority that accepts or rejects the write, and the
// session-owned store is only ever updated once Events has already accepted
// the exact same identity. If that authoritative write path regressed --
// for example, an Events rejection no longer failing the publish, or the
// store and Events disagreeing on sequence -- this session would produce an
// incomplete, gapped, or duplicated public response-event history instead of
// the complete, contiguous, uniquely identified one asserted below. This is
// a real end-to-end regression guard for that delegation, complementing the
// focused unit/composition coverage in mirror_test.go (which proves the
// underlying Events records directly) and pkg/wire's shared-instance
// composition tests (which prove canonical construction): the Events root
// itself is process-internal wiring with no public accessor, by design, so
// this functional pass is what proves the customer-observable guarantee
// still holds when driven through the real running process rather than an
// isolated construction call.
func TestFactoryResponseEventsSurviveTheEventsAuthoritativePublishPath(t *testing.T) {
	t.Parallel()

	loaded := loadCodexPartialStreamGoldenCase(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(
		t,
		dir,
		"worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"events authoritative publish path"}`))

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
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}
	if len(responseEvents) == 0 {
		t.Fatal("session produced zero public Factory response events")
	}
	assertResponseEventsAscendingSequence(t, responseEvents)

	seenEventIDs := make(map[string]bool, len(responseEvents))
	for _, event := range responseEvents {
		if event.EventId == "" {
			t.Fatalf("response event sequence %d has an empty EventId", event.Sequence)
		}
		if seenEventIDs[event.EventId] {
			t.Fatalf("response event id %q observed more than once (identity must be assigned exactly once, matching the injected Events root's own idempotency)", event.EventId)
		}
		seenEventIDs[event.EventId] = true
	}
}
