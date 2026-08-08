package provider_sessions_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	_, work, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		2*time.Minute,
	)

	if got := support.CountWorkAtCustomerState(work, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("large rollout completed work = %d, want 1; listed=%#v", got, work)
	}
	if got := support.CountWorkAtCustomerState(work, support.WorkCustomerLocation("task", "failed")); got != 0 {
		t.Fatalf("large rollout failed work = %d, want 0; listed=%#v", got, work)
	}
	if got := runner.calls.Load(); got != 1 {
		t.Fatalf("large rollout provider command calls = %d, want 1", got)
	}
	assertAuthoritativeLargeRolloutCompletion(t, events)
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
	file, err := os.Open(r.path)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	defer file.Close()

	buffer := make([]byte, largeRolloutStreamChunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return platformprocess.CommandResult{}, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			observer(platformprocess.OutputStreamStdout, buffer[:count])
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			return platformprocess.CommandResult{ExitCode: 0}, nil
		}
		return platformprocess.CommandResult{}, readErr
	}
}

var _ platformprocess.CommandRunner = (*largeRolloutCommandRunner)(nil)
