package response_events

import (
	"fmt"
	"path/filepath"
	"strconv"
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

const codexPartialStreamGoldenCase = "success"

// TestAPIResponseEventSSEStreamsRetainedThenLiveEvents proves the public
// Response Event SSE API first delivers retained matching records in ascending
// FactoryResponseEvent.sequence and then continues with later live matching
// records on the same connection without reordering retained catch-up history.
func TestAPIResponseEventSSEStreamsRetainedThenLiveEvents(t *testing.T) {
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
	providerResult := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	runner := testutil.NewProviderCommandRunner(providerResult, providerResult)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	firstWorkName := "retained-then-live-first-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &firstWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "first invocation for retained history",
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)

	sessionID := factorysessions.DefaultSessionID
	retained := support.GetFactoryResponseEventsAt(t, server.URL(), sessionID)
	if len(retained) < 2 {
		t.Fatalf(
			"retained Response Event count = %d, want at least 2 after first invocation",
			len(retained),
		)
	}
	assertResponseEventsAscendingSequence(t, retained)

	stream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), sessionID),
	)

	retainedFromStream := collectResponseEventStreamUntilCount(
		t,
		stream,
		len(retained),
		10*time.Second,
	)
	assertResponseEventFramesMatchRetainedCatchUp(t, retained, retainedFromStream)

	secondWorkName := "retained-then-live-second-work"
	support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &secondWorkName,
		WorkTypeName: "task",
		Payload: map[string]string{
			"title": "second invocation for live continuation",
		},
	})

	maxRetainedSequence := retained[len(retained)-1].Sequence
	liveFromStream := collectResponseEventStreamUntilQuietAfterSequence(
		t,
		stream,
		maxRetainedSequence,
		20*time.Second,
	)
	support.WaitForTerminalStatus(t, server.URL(), 20*time.Second)
	stream.Close()

	if len(liveFromStream) == 0 {
		t.Fatal("live continuation delivered zero Response Events on the same SSE connection")
	}
	for _, frame := range liveFromStream {
		if frame.Event.Sequence <= maxRetainedSequence {
			t.Fatalf(
				"live continuation event sequence %d is not after retained max %d",
				frame.Event.Sequence,
				maxRetainedSequence,
			)
		}
	}
	assertResponseEventFramesAscendingSequence(t, liveFromStream)
	assertResponseEventFramesSSEIDMatchesSequence(
		t,
		append(retainedFromStream, liveFromStream...),
	)

	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner calls = %d, want 2 invocations", runner.CallCount())
	}
}

func loadCodexPartialStreamGoldenCase(t *testing.T) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath(
			string(modelprovider.ProviderCodex),
			codexPartialStreamGoldenCase,
		)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", codexPartialStreamGoldenCase, err)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityPartialStream {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityPartialStream,
		)
	}
	return loaded
}

func collectResponseEventStreamUntilCount(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	wantCount int,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	for len(collected) < wantCount {
		select {
		case <-deadline.C:
			t.Fatalf(
				"timed out collecting %d Response Event frames from stream; got %d within %s",
				wantCount,
				len(collected),
				timeout,
			)
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			collected = append(collected, frame)
		}
	}
	return collected
}

func collectResponseEventStreamUntilQuietAfterSequence(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	afterSequence int64,
	timeout time.Duration,
) []support.FactoryResponseEventFrame {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	var collected []support.FactoryResponseEventFrame
	var quiet *time.Timer
	var quietC <-chan time.Time
	for {
		select {
		case <-deadline.C:
			return collected
		case <-quietC:
			return collected
		default:
			frame, ok := stream.TryNextFrame(50 * time.Millisecond)
			if !ok {
				continue
			}
			if frame.Event.Sequence <= afterSequence {
				t.Fatalf(
					"live continuation frame sequence %d is not after acknowledged sequence %d",
					frame.Event.Sequence,
					afterSequence,
				)
			}
			collected = append(collected, frame)
			if quiet == nil {
				quiet = time.NewTimer(250 * time.Millisecond)
			} else {
				if !quiet.Stop() {
					select {
					case <-quiet.C:
					default:
					}
				}
				quiet.Reset(250 * time.Millisecond)
			}
			quietC = quiet.C
		}
	}
}

func assertResponseEventsAscendingSequence(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	previousSequence := int64(-1)
	for index, event := range events {
		if event.Sequence <= previousSequence {
			t.Fatalf(
				"Response Event %d (%s) sequence %d precedes previous sequence %d",
				index,
				event.EventId,
				event.Sequence,
				previousSequence,
			)
		}
		previousSequence = event.Sequence
	}
}

func assertResponseEventFramesAscendingSequence(
	t *testing.T,
	frames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	previousSequence := int64(-1)
	for index, frame := range frames {
		if frame.Event.Sequence <= previousSequence {
			t.Fatalf(
				"Response Event frame %d (%s) sequence %d precedes previous sequence %d",
				index,
				frame.Event.EventId,
				frame.Event.Sequence,
				previousSequence,
			)
		}
		previousSequence = frame.Event.Sequence
	}
}

func assertResponseEventFramesMatchRetainedCatchUp(
	t *testing.T,
	wantRetained []factoryapi.FactoryResponseEvent,
	gotFrames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	if len(gotFrames) != len(wantRetained) {
		t.Fatalf(
			"retained catch-up frame count = %d, want %d",
			len(gotFrames),
			len(wantRetained),
		)
	}
	for index := range wantRetained {
		want := wantRetained[index]
		got := gotFrames[index].Event
		if got.EventId != want.EventId {
			t.Fatalf(
				"retained catch-up event at index %d = %q, want %q",
				index,
				got.EventId,
				want.EventId,
			)
		}
		if got.Sequence != want.Sequence {
			t.Fatalf(
				"retained catch-up sequence at index %d for event %q = %d, want %d",
				index,
				got.EventId,
				got.Sequence,
				want.Sequence,
			)
		}
	}
}

func assertResponseEventFramesSSEIDMatchesSequence(
	t *testing.T,
	frames []support.FactoryResponseEventFrame,
) {
	t.Helper()

	for _, frame := range frames {
		wantID := strconv.FormatInt(frame.Event.Sequence, 10)
		if frame.SSEID != wantID {
			t.Fatalf(
				"Response Event SSE id = %q for event %q sequence %d, want decimal sequence %q",
				frame.SSEID,
				frame.Event.EventId,
				frame.Event.Sequence,
				wantID,
			)
		}
		if frame.Event.Kind == factoryapi.FactoryResponseEventKindStreamGap && frame.Event.Sequence == 0 {
			continue
		}
		if frame.SSEID != fmt.Sprint(frame.Event.Sequence) {
			t.Fatalf(
				"Response Event SSE id %q does not match event sequence %d",
				frame.SSEID,
				frame.Event.Sequence,
			)
		}
	}
}
