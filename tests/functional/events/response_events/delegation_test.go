package response_events

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

// TestFactoryResponseEventDeliveryDivergesWhenEventsRetentionIsTighterThanLocal
// proves SubscribeFactoryResponseEvents genuinely reads delivered content
// back through the injected Events root rather than only from Factory
// Sessions' own locally retained copy. It runs the identical golden Codex
// session twice through the exact customer-facing response-event HTTP
// endpoint, changing only edges.EventsMaxRetainedRecordsPerTopic between
// runs: with the production default, every published event is retained and
// delivered with real content; with the process-scoped Events root's
// per-topic retention tightened below the number of events this session
// publishes -- while Factory Sessions' own response-event retention limits
// stay at their large default, so this store's own tiered retention would
// otherwise still consider every position deliverable -- delivery must
// instead surface each of those positions as an explicit STREAM_GAP
// reasoned "events_authority_unavailable" rather than falling back to this
// store's local bytes. If Subscribe reverted to reading only its own local
// copy -- the exact regression this test guards against -- the tightened
// run would deliver identical real content to the baseline run and this
// test would fail; a public HTTP contract test that only asserts ascending
// unique sequence numbers (as
// TestFactoryResponseEventsSurviveTheEventsAuthoritativePublishPath does for
// the write path) would not detect that regression on the read path, which
// is exactly the coverage gap this test closes.
func TestFactoryResponseEventDeliveryDivergesWhenEventsRetentionIsTighterThanLocal(t *testing.T) {
	t.Parallel()

	baseline := runCodexGoldenSessionAndListResponseEvents(t, 0)
	if len(baseline) == 0 {
		t.Fatal("baseline run (production default Events retention) produced zero public Factory response events")
	}
	assertResponseEventsAscendingSequence(t, baseline)
	for _, event := range baseline {
		if event.Kind == factoryapi.FactoryResponseEventKindStreamGap {
			t.Fatalf(
				"baseline run (production default Events retention) unexpectedly delivered a STREAM_GAP event at sequence %d",
				event.Sequence,
			)
		}
	}

	tightened := runCodexGoldenSessionAndListResponseEvents(t, 2)
	if len(tightened) != len(baseline) {
		t.Fatalf(
			"tightened-Events-retention run delivered %d events, want %d (same total as baseline; substitution must replace content, not drop or duplicate positions)",
			len(tightened),
			len(baseline),
		)
	}
	for index, event := range tightened {
		if event.Kind != factoryapi.FactoryResponseEventKindStreamGap {
			t.Fatalf(
				"tightened-Events-retention run delivered real content at index %d (sequence %d, kind %s); if Subscribe stopped reading through the injected Events root and reverted to this store's own locally retained copy, this event would still show real content instead of surfacing the gap Events' own tighter retention created",
				index,
				event.Sequence,
				event.Kind,
			)
		}
		gapUnion, err := event.Payload.AsFactoryResponseEventStreamGapPayload()
		if err != nil {
			t.Fatalf("decode STREAM_GAP union payload at index %d: %v", index, err)
		}
		gapPayload, err := gapUnion.AsFactoryResponseEventStreamGapPayload0()
		if err != nil {
			t.Fatalf("decode STREAM_GAP payload at index %d: %v", index, err)
		}
		if gapPayload.Reason == nil || *gapPayload.Reason != "events_authority_unavailable" {
			t.Fatalf("STREAM_GAP reason at index %d = %#v, want events_authority_unavailable", index, gapPayload.Reason)
		}
	}
}

// runCodexGoldenSessionAndListResponseEvents runs the codex "success" golden
// session fixture through root.BuildProcess (via
// support.StartFunctionalAPIServer) and the real customer response-event
// HTTP endpoint, with only the provider command boundary and Events'
// per-topic retention cap replaced through edges.Edges, and returns the
// resulting public Factory response events for the default session.
// eventsMaxRetainedRecordsPerTopic of 0 keeps the production default.
func runCodexGoldenSessionAndListResponseEvents(
	t *testing.T,
	eventsMaxRetainedRecordsPerTopic int,
) []factoryapi.FactoryResponseEvent {
	t.Helper()

	loaded := loadCodexPartialStreamGoldenCase(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(
		t,
		dir,
		"worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, loaded.Process.Model),
	)

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner:            runner,
			EventsMaxRetainedRecordsPerTopic: eventsMaxRetainedRecordsPerTopic,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	workName := "events-authority-delegation-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "publish response events for the Events-authority delegation proof",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)

	return support.GetFactoryResponseEventsAt(t, server.URL(), factorysessions.DefaultSessionID)
}
