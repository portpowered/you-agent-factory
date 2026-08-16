package provider_sessions_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	largeRolloutSessionID       = "large-rollout-functional-session"
	largeRolloutTargetBytes     = int64(256 << 20)
	largeRolloutPaddingBytes    = 256 << 10
	largeRolloutStreamChunkSize = 64 << 10
)

// TestLargeRolloutNeverFailsWithoutCause generates a temporary multi-hundred-MB
// Codex JSONL rollout, streams it through the command edge, and observes the
// customer-visible Work and Factory Event terminal projections. The test uses
// root.BuildProcess and Process.Execute through the shared functional harness;
// only the provider command edge is replaced.
func TestLargeRolloutNeverFailsWithoutCause(t *testing.T) {
	support.SkipLongFunctional(t, "large rollout regression belongs to the non-short functional lane")

	rolloutPath := writeLargeCodexRollout(t)
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(
		t,
		dir,
		"worker",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"),
	)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"large rollout terminal classification"}`))

	runner := &largeRolloutCommandRunner{path: rolloutPath}
	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:      server.Start,
		ProviderCommandRunner: runner,
	})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := server.WaitForURL(t)
	liveSession := support.GetDefaultSession(t, baseURL)
	eventStream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(baseURL, liveSession.Id),
	)
	events := readLargeRolloutEventsUntilDispatchResponse(t, eventStream)
	work := support.ListDefaultSessionWork(t, baseURL)
	session := support.GetDefaultSession(t, baseURL)
	daemon.Stop(t)

	if got := support.CountWorkAtCustomerState(work, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("large rollout completed work = %d, want 1; listed=%#v", got, work)
	}
	if got := support.CountWorkAtCustomerState(work, support.WorkCustomerLocation("task", "failed")); got != 0 {
		t.Fatalf("large rollout failed work = %d, want 0; listed=%#v", got, work)
	}
	if got := runner.calls.Load(); got != 1 {
		t.Fatalf("large rollout provider command calls = %d, want 1", got)
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("large rollout session progress = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
	assertAuthoritativeLargeRolloutCompletion(t, events)
}

func readLargeRolloutEventsUntilDispatchResponse(
	t *testing.T,
	stream *support.FactoryEventStream,
) []factoryapi.FactoryEvent {
	t.Helper()

	// The Factory Event stream is the synchronization primitive here. It
	// replays retained history and then blocks for the live terminal event, so
	// this test does not poll status or infer completion from a timeout window.
	const observationTimeout = 2 * time.Minute
	events := make([]factoryapi.FactoryEvent, 0, 16)
	for {
		event := stream.NextEvent(observationTimeout)
		events = append(events, event)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		if _, err := event.Payload.AsDispatchResponseEventPayload(); err != nil {
			t.Fatalf("decode large-rollout dispatch response: %v", err)
		}
		return events
	}
}

func writeLargeCodexRollout(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "rollout-"+largeRolloutSessionID+".jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create generated rollout: %v", err)
	}
	defer func() { _ = file.Close() }()
	writer := bufio.NewWriterSize(file, largeRolloutStreamChunkSize)
	written := int64(0)
	write := func(record []byte) {
		t.Helper()
		if _, err := writer.Write(record); err != nil {
			t.Fatalf("write generated rollout: %v", err)
		}
		written += int64(len(record))
	}

	write([]byte(`{"type":"thread.started","thread_id":"` + largeRolloutSessionID + `"}` + "\n"))
	filler, err := json.Marshal(map[string]any{
		"type": "item.updated",
		"item": map[string]any{
			"id":   "large-rollout-progress",
			"type": "agent_message",
			"text": strings.Repeat("x", largeRolloutPaddingBytes),
		},
	})
	if err != nil {
		t.Fatalf("marshal generated rollout record: %v", err)
	}
	filler = append(filler, '\n')
	for written < largeRolloutTargetBytes {
		write(filler)
	}
	write([]byte(`{"type":"item.completed","item":{"id":"large-rollout-final","type":"agent_message","text":"large rollout completed with authoritative Codex evidence COMPLETE"}}` + "\n"))

	if err := writer.Flush(); err != nil {
		t.Fatalf("flush generated rollout: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close generated rollout: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat generated rollout: %v", err)
	}
	if info.Size() < largeRolloutTargetBytes {
		t.Fatalf("generated rollout size = %d, want at least %d", info.Size(), largeRolloutTargetBytes)
	}
	return path
}

func assertAuthoritativeLargeRolloutCompletion(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	var successfulResponses int
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse &&
			event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		response, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode large-rollout provider response: %v", err)
		}
		if response.Outcome != factoryapi.InferenceOutcomeSucceeded {
			t.Fatalf("large-rollout provider outcome = %q, want succeeded; response=%#v", response.Outcome, response)
		}
		if response.ProviderSession == nil || response.ProviderSession.Id == nil ||
			*response.ProviderSession.Id != largeRolloutSessionID {
			t.Fatalf("large-rollout provider session = %#v, want authoritative session %q", response.ProviderSession, largeRolloutSessionID)
		}
		if response.Response == nil || !strings.Contains(*response.Response, "authoritative Codex evidence") {
			t.Fatalf("large-rollout provider response = %#v, want final completion evidence", response.Response)
		}
		successfulResponses++
	}
	if successfulResponses != 1 {
		t.Fatalf("successful large-rollout provider responses = %d, want 1", successfulResponses)
	}
}

type largeRolloutCommandRunner struct {
	path  string
	calls atomic.Int32
}

func (r *largeRolloutCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("large rollout command runner requires streaming")
}

func (r *largeRolloutCommandRunner) RunStreaming(
	ctx context.Context,
	_ platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	if observer == nil {
		return platformprocess.CommandResult{}, errors.New("large rollout command runner requires an output observer")
	}
	r.calls.Add(1)
	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil, nil)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	return runner.RunStreaming(ctx, platformprocess.CommandRequest{
		Command: os.Args[0],
		Args: []string{
			"-test.run=TestLargeRolloutHelperProcess",
			"--",
			"stream",
		},
		Env: append(
			os.Environ(),
			"GO_WANT_LARGE_ROLLOUT_HELPER=1",
			"LARGE_ROLLOUT_PATH="+r.path,
		),
	}, observer)
}

func TestLargeRolloutHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LARGE_ROLLOUT_HELPER") != "1" {
		return
	}
	path := os.Getenv("LARGE_ROLLOUT_PATH")
	if path == "" {
		t.Fatal("missing LARGE_ROLLOUT_PATH")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open generated rollout: %v", err)
	}
	defer file.Close()
	if _, err := io.Copy(os.Stdout, file); err != nil {
		t.Fatalf("stream generated rollout: %v", err)
	}
	os.Exit(0)
}

var _ platformprocess.CommandRunner = (*largeRolloutCommandRunner)(nil)
