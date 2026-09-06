package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func fscp03ChildSource(label string) factorysessions.Source {
	return fscp03InlineSource(fmt.Sprintf(`return (async function () {
  return await agent.run({
    prompt: %q,
    label: %q,
    modelProvider: "codex",
    model: "gpt-5-codex"
  });
})();`, label, label))
}

type fscp03BarrierRunner struct {
	gate    chan struct{}
	started chan struct{}
}

func newFSCP03BarrierRunner() *fscp03BarrierRunner {
	return &fscp03BarrierRunner{gate: make(chan struct{}), started: make(chan struct{}, 32)}
}

func (runner *fscp03BarrierRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	select {
	case <-runner.gate:
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("fscp03 barrier COMPLETE")}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func (runner *fscp03BarrierRunner) WaitStarted(ctx context.Context, count int) error {
	for range count {
		select {
		case <-runner.started:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fscp03ObservationTimeout):
			return fmt.Errorf("waiting for provider dispatch %d", count)
		}
	}
	return nil
}

func (runner *fscp03BarrierRunner) Release() { close(runner.gate) }

func (runner *fscp03BarrierRunner) Reset() {
	runner.gate = make(chan struct{})
}

var _ platformprocess.CommandRunner = (*fscp03BarrierRunner)(nil)

type fscp03ControlRunner struct {
	started chan struct{}
}

func newFSCP03ControlRunner() *fscp03ControlRunner {
	return &fscp03ControlRunner{started: make(chan struct{}, 32)}
}

func (runner *fscp03ControlRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	select {
	case runner.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return platformprocess.CommandResult{}, ctx.Err()
}

var _ platformprocess.CommandRunner = (*fscp03ControlRunner)(nil)

type fscp03LiveRunner struct {
	mu      sync.Mutex
	gate    chan struct{}
	started chan struct{}
}

func newFSCP03LiveRunner() *fscp03LiveRunner {
	return &fscp03LiveRunner{started: make(chan struct{}, 32)}
}

func (runner *fscp03LiveRunner) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	gate := runner.gate
	runner.mu.Unlock()
	if gate != nil {
		select {
		case runner.started <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("fscp03 live COMPLETE")}, nil
}

func (runner *fscp03LiveRunner) Hold() {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.gate = make(chan struct{})
}

func (runner *fscp03LiveRunner) Release() {
	runner.mu.Lock()
	gate := runner.gate
	runner.gate = nil
	runner.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

func (runner *fscp03LiveRunner) WaitStarted(ctx context.Context, count int) error {
	for range count {
		select {
		case <-runner.started:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fscp03ObservationTimeout):
			return fmt.Errorf("waiting for live provider dispatch %d", count)
		}
	}
	return nil
}

var _ platformprocess.CommandRunner = (*fscp03LiveRunner)(nil)

type fscp03HTTPInvocationOutcome struct {
	response factoryapi.InvocationResponse
	err      error
}

func postFSCP03Invocation(ctx context.Context, baseURL, sessionID, text string) (factoryapi.InvocationResponse, error) {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    ptr(factoryapi.WorkContent{part}),
	})
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

func ptr[T any](value T) *T { return &value }

func assertFSCP03HTTPInvocation(t *testing.T, response factoryapi.InvocationResponse) {
	t.Helper()
	if response.Status != factoryapi.InvocationTerminalStatusCompleted || strings.TrimSpace(response.RequestId) == "" || strings.TrimSpace(response.TraceId) == "" {
		t.Fatalf("HTTP invocation response = %#v, want COMPLETED request/trace identity", response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) == 0 {
		t.Fatalf("HTTP invocation primary result = %#v, want result content", response.PrimaryResult)
	}
}

func assertFSCP03LiveFactoryEvents(t *testing.T, baseURL, firstID, secondID string) {
	t.Helper()
	first := support.GetFactoryEventsForSessionAt(t, baseURL, firstID)
	second := support.GetFactoryEventsForSessionAt(t, baseURL, secondID)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("live Factory Events first=%d second=%d, want events for both sessions", len(first), len(second))
	}
	firstWorkIDs := make(map[string]struct{})
	firstSessionEvents := 0
	for _, event := range first {
		if event.Context.SessionId != nil {
			firstSessionEvents++
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != firstID && *event.Context.SessionId != factorysessions.DefaultSessionID {
			t.Fatalf("first Factory Event context = %#v, want session %q", event.Context, firstID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				firstWorkIDs[workID] = struct{}{}
			}
		}
	}
	if len(firstWorkIDs) == 0 {
		t.Fatalf("first Factory Events = %#v, want Work lineage", first)
	}
	if firstSessionEvents == 0 {
		t.Fatalf("first Factory Events = %#v, want at least one session-correlated event", first)
	}
	secondSessionEvents := 0
	for _, event := range second {
		if event.Context.SessionId != nil {
			secondSessionEvents++
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != secondID {
			t.Fatalf("second Factory Event context = %#v, want session %q", event.Context, secondID)
		}
		if event.Context.WorkIds != nil {
			for _, workID := range *event.Context.WorkIds {
				if _, shared := firstWorkIDs[workID]; shared {
					t.Fatalf("Work identity %q crossed live sessions", workID)
				}
			}
		}
	}
	if secondSessionEvents == 0 {
		t.Fatalf("second Factory Events = %#v, want at least one session-correlated event", second)
	}
}

func assertFSCP03LiveResponseEvents(t *testing.T, baseURL, firstID, secondID string) {
	t.Helper()
	first := support.GetFactoryResponseEventsAt(t, baseURL, firstID)
	second := support.GetFactoryResponseEventsAt(t, baseURL, secondID)
	if len(first) == 0 || len(second) == 0 {
		t.Fatalf("live Response Events first=%d second=%d, want events for both sessions", len(first), len(second))
	}
	firstEventIDs := make(map[string]struct{}, len(first))
	firstDispatchIDs := make(map[string]struct{}, len(first))
	for _, event := range first {
		if event.FactorySessionId != firstID || strings.TrimSpace(event.EventId) == "" {
			t.Fatalf("first Response Event = %#v, want session-scoped identity", event)
		}
		firstEventIDs[event.EventId] = struct{}{}
		if event.DispatchId != nil {
			firstDispatchIDs[*event.DispatchId] = struct{}{}
		}
	}
	if len(firstDispatchIDs) == 0 {
		t.Fatalf("first Response Events = %#v, want dispatch identity", first)
	}
	for _, event := range second {
		if event.FactorySessionId != secondID {
			t.Fatalf("second Response Event = %#v, want session %q", event, secondID)
		}
		if _, shared := firstEventIDs[event.EventId]; shared {
			t.Fatalf("Response Event identity %q crossed live sessions", event.EventId)
		}
		if event.DispatchId != nil {
			if _, shared := firstDispatchIDs[*event.DispatchId]; shared {
				t.Fatalf("dispatch identity %q crossed live sessions", *event.DispatchId)
			}
		}
	}
}

func collectFSCP03ResponseFrames(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	sessionID string,
) []support.FactoryResponseEventFrame {
	t.Helper()
	var frames []support.FactoryResponseEventFrame
	deadline := time.NewTimer(fscp03ObservationTimeout)
	defer deadline.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatalf("timed out collecting response events for session %q; got %d frames", sessionID, len(frames))
		default:
		}
		frame, ok := stream.TryNextFrame(time.Second)
		if !ok {
			t.Fatalf("response event stream for session %q closed before MESSAGE completion; got %d frames", sessionID, len(frames))
		}
		if frame.Event.FactorySessionId != sessionID {
			t.Fatalf("response frame session = %q, want %q", frame.Event.FactorySessionId, sessionID)
		}
		frames = append(frames, frame)
		if frame.Event.Kind == factoryapi.FactoryResponseEventKindMessage && frame.Event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
			return frames
		}
	}
}

func assertFSCP03DisjointHTTPResponseFrames(t *testing.T, first, second []support.FactoryResponseEventFrame) {
	t.Helper()
	firstEventIDs := make(map[string]struct{}, len(first))
	firstDispatchIDs := make(map[string]struct{}, len(first))
	for _, frame := range first {
		firstEventIDs[frame.Event.EventId] = struct{}{}
		if frame.Event.DispatchId != nil {
			firstDispatchIDs[*frame.Event.DispatchId] = struct{}{}
		}
	}
	if len(firstDispatchIDs) == 0 {
		t.Fatal("first concurrent response stream had no dispatch identity")
	}
	for _, frame := range second {
		if _, shared := firstEventIDs[frame.Event.EventId]; shared {
			t.Fatalf("concurrent Response Event identity %q crossed streams", frame.Event.EventId)
		}
		if frame.Event.DispatchId != nil {
			if _, shared := firstDispatchIDs[*frame.Event.DispatchId]; shared {
				t.Fatalf("concurrent dispatch identity %q crossed streams", *frame.Event.DispatchId)
			}
		}
	}
}

func scaffoldFSCP03ProbeFactory(t *testing.T) string {
	t.Helper()
	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "fscp03-probe",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}
