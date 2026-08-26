package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren proves the
// shared resource gate at the public JavaScript Factory Session boundary. One
// child is held at the injected mock-worker command edge, the second child
// waits on reviewers capacity one, and a live increase admits it in the same
// durable session with exactly two completed child dispatches.
func TestJavaScriptLiveResourceCapacityIncreaseWakesWaitingChildren(t *testing.T) {
	t.Parallel()
	provider := newLiveCapacityJavaScriptProviderRunner()
	dir := scaffoldLiveCapacityFactory(t, 1)
	support.WriteAgentConfig(t, dir, liveCapacityWorker, "---\n"+
		"type: MODEL_WORKER\n"+
		"---\n"+
		"Use the capacity worker for JavaScript children.\n")
	homeDir := writeLiveCapacityJavaScriptGlobalConfig(t)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Env: append(os.Environ(),
			"HOME="+homeDir,
			"USERPROFILE="+homeDir,
		),
		Edges: serviceedges.Edges{ProviderCommandRunner: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	started := startLiveCapacityJavaScriptWorkflow(t, server.URL(), liveCapacityJavaScriptWorkflow)
	if started.SessionId == "" {
		t.Fatal("JavaScript capacity workflow has no durable session ID")
	}
	responseStream := support.OpenFactoryResponseEventStreamAt(
		t,
		support.SessionResponseEventsURL(server.URL(), started.SessionId),
	)
	provider.waitForCall(t, 1)
	before := readLiveCapacityDurableSession(t, server.URL(), started.SessionId)

	capacity := runLiveCapacityCLI(t, dir, server.URL(), started.SessionId, liveCapacityResourceID, 2, 0, "javascript-capacity-raise", "raise JavaScript capacity")
	if capacity.Outcome != factoryapi.FactorySessionResourceCapacityOutcome("APPLIED") ||
		capacity.SessionId != started.SessionId || capacity.PreviousCapacity != 1 ||
		capacity.EffectiveCapacity != 2 || capacity.InUseCount != 1 || capacity.AvailableCount != 1 ||
		capacity.Revision != 1 {
		t.Fatalf("JavaScript capacity response = %#v, want applied reviewers 1->2 in same session", capacity)
	}
	provider.waitForCall(t, 2)
	afterRaise := readLiveCapacityDurableSession(t, server.URL(), started.SessionId)
	if afterRaise.SessionId != before.SessionId {
		t.Fatalf("JavaScript Factory Session id changed from %q to %q after live raise", before.SessionId, afterRaise.SessionId)
	}

	close(provider.releaseBlocked)
	terminalEvent := waitForLiveCapacityJavaScriptTerminal(
		t,
		responseStream,
		server.URL(),
		started.SessionId,
		provider,
		2,
	)
	if terminalEvent.FactorySessionId != started.SessionId {
		t.Fatalf(
			"JavaScript terminal response event session = %q, want %q",
			terminalEvent.FactorySessionId,
			started.SessionId,
		)
	}
	terminal := readLiveCapacityDurableSession(t, server.URL(), started.SessionId)
	if terminal.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("JavaScript durable session status = %q, want SUCCEEDED", terminal.Status)
	}
	providerState := provider.snapshot()
	if providerState.calls != 2 || providerState.completed != 2 || providerState.active != 0 || providerState.peak != 2 {
		t.Fatalf(
			"JavaScript provider state = %#v, want exactly two calls, two completions, no active effects, and peakActive=2",
			providerState,
		)
	}
	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+started.SessionId+"/dispatches",
	)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("JavaScript dispatch count = %d, want two resource-bound children", len(dispatches.Dispatches))
	}
	seenIDs := make(map[string]struct{}, len(dispatches.Dispatches))
	seenLabels := make(map[string]struct{}, len(dispatches.Dispatches))
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED || dispatch.Javascript == nil {
			t.Fatalf("JavaScript dispatch = %#v, want completed JavaScript projection", dispatch)
		}
		if _, duplicate := seenIDs[dispatch.Id]; duplicate || dispatch.Id == "" {
			t.Fatalf("JavaScript dispatch IDs contain duplicate or empty ID: %q", dispatch.Id)
		}
		seenIDs[dispatch.Id] = struct{}{}
		if dispatch.Label == nil || *dispatch.Label == "" {
			t.Fatalf("JavaScript dispatch %q has no public child label", dispatch.Id)
		}
		seenLabels[*dispatch.Label] = struct{}{}
	}
	for _, label := range []string{"javascript-child-one", "javascript-child-two"} {
		if _, ok := seenLabels[label]; !ok {
			t.Fatalf("JavaScript dispatch labels = %#v, missing %q", seenLabels, label)
		}
	}
}

func startLiveCapacityJavaScriptWorkflow(
	t *testing.T,
	serverURL, workflowSource string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	// API-owned exception: durable JavaScript Factory Session execution is
	// exposed through POST /factory-sessions/async (and MCP), with no
	// equivalent public CLI command. Ordinary Work submission and live
	// capacity controls in this feature use Process.Execute below.
	dialect := "you-workflow-v1"
	inlineSource := factoryapi.FactoryOrchestratorJavaScriptInlineSource{
		Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
		Inline:   workflowSource,
	}
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "javascript-live-capacity-workflow",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect:      &dialect,
				InlineSource: inlineSource,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal JavaScript capacity workflow: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-sessions/async"
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build JavaScript capacity workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start JavaScript capacity workflow: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read JavaScript capacity workflow response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start JavaScript capacity workflow status = %d, want 200\n%s", response.StatusCode, body)
	}
	var started factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(body, &started); err != nil {
		t.Fatalf("decode JavaScript capacity workflow response: %v\n%s", err, body)
	}
	return started
}

func readLiveCapacityDurableSession(
	t *testing.T,
	serverURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/"+sessionID,
	)
	session, err := response.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode JavaScript durable Factory Session: %v", err)
	}
	return session
}

func waitForLiveCapacityJavaScriptTerminal(
	t *testing.T,
	stream *support.FactoryResponseEventStream,
	serverURL, sessionID string,
	provider *liveCapacityJavaScriptProviderRunner,
	wantRuns int,
) factoryapi.FactoryResponseEvent {
	t.Helper()
	completedRuns := 0
	frames := make([]factoryapi.FactoryResponseEvent, 0, wantRuns)
	for {
		result := stream.TryNextFrameResult(liveCapacityTestTimeout)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf(
				"JavaScript workflow response stream ended before %d completed RUN events: %s; delivered frames=%s; lifecycle=%s",
				wantRuns,
				result.Diagnostic(),
				summarizeLiveCapacityResponseEvents(frames),
				liveCapacityJavaScriptLifecycleEvidence(serverURL, sessionID, provider),
			)
		}
		event := result.Frame.Event
		frames = append(frames, event)
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindRun:
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedRuns++
				if completedRuns >= wantRuns {
					// The final RUN frame precedes durable session finalization. The
					// response stream closes when the session-owned execution has
					// drained, providing a deterministic lifecycle barrier without
					// polling the durable projection.
					stream.WaitClosed(liveCapacityTestTimeout)
					return event
				}
			}
		case factoryapi.FactoryResponseEventKindError,
			factoryapi.FactoryResponseEventKindSession:
			if event.Phase == factoryapi.FactoryResponseEventPhaseFailed ||
				event.Phase == factoryapi.FactoryResponseEventPhaseCanceled {
				t.Fatalf("JavaScript workflow terminal response event = %s/%s", event.Kind, event.Phase)
			}
		}
	}
}

func summarizeLiveCapacityResponseEvents(events []factoryapi.FactoryResponseEvent) string {
	if len(events) == 0 {
		return "none"
	}
	summaries := make([]string, 0, len(events))
	for _, event := range events {
		summaries = append(
			summaries,
			fmt.Sprintf("sequence=%d kind=%s phase=%s run=%q", event.Sequence, event.Kind, event.Phase, event.RunId),
		)
	}
	return strings.Join(summaries, "; ")
}

func liveCapacityJavaScriptLifecycleEvidence(
	serverURL, sessionID string,
	provider *liveCapacityJavaScriptProviderRunner,
) string {
	providerState := provider.snapshot()
	sessionResponse, sessionErr := readLiveCapacityLifecycleJSON[factoryapi.FactorySessionGetResponse](
		serverURL,
		"/factory-sessions/"+sessionID,
	)
	sessionEvidence := fmt.Sprintf("error=%v", sessionErr)
	if sessionErr == nil {
		session, err := sessionResponse.AsFactorySessionDurableReadModel()
		if err != nil {
			sessionEvidence = fmt.Sprintf("decode-error=%v", err)
		} else {
			phase := ""
			if session.Phase != nil {
				phase = *session.Phase
			}
			sessionEvidence = fmt.Sprintf(
				"status=%s phase=%q progress=%#v",
				session.Status,
				phase,
				session.Progress,
			)
		}
	}

	dispatches, dispatchErr := readLiveCapacityLifecycleJSON[factoryapi.ListFactorySessionDispatchesResponse](
		serverURL,
		"/factory-sessions/"+sessionID+"/dispatches",
	)
	dispatchEvidence := fmt.Sprintf("error=%v", dispatchErr)
	if dispatchErr == nil {
		statuses := make([]string, 0, len(dispatches.Dispatches))
		for _, dispatch := range dispatches.Dispatches {
			label := support.StringPointerValue(dispatch.Label)
			if label == "" {
				label = dispatch.Id
			}
			statuses = append(statuses, fmt.Sprintf("%s=%s", label, dispatch.Status))
		}
		dispatchEvidence = fmt.Sprintf("count=%d statuses=%s", len(statuses), strings.Join(statuses, ","))
	}

	return fmt.Sprintf(
		"session={%s}; dispatches={%s}; provider={calls=%d completed=%d active=%d peak=%d}",
		sessionEvidence,
		dispatchEvidence,
		providerState.calls,
		providerState.completed,
		providerState.active,
		providerState.peak,
	)
}

func readLiveCapacityLifecycleJSON[T any](serverURL, path string) (T, error) {
	var value T
	request, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		strings.TrimSuffix(serverURL, "/")+path,
		nil,
	)
	if err != nil {
		return value, err
	}
	client := http.Client{Timeout: time.Second}
	response, err := client.Do(request)
	if err != nil {
		return value, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return value, err
	}
	if response.StatusCode != http.StatusOK {
		return value, fmt.Errorf("HTTP status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, &value); err != nil {
		return value, err
	}
	return value, nil
}

func writeLiveCapacityJavaScriptGlobalConfig(t *testing.T) string {
	t.Helper()
	homeDir := t.TempDir()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("create JavaScript capacity global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "codex", "workerModel": "mock-capacity-model"},
  "workerPresets": [{"id": "capacity-worker", "modelProvider": "codex", "model": "mock-capacity-model"}]
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write JavaScript capacity global config: %v", err)
	}
	return homeDir
}

type liveCapacityJavaScriptProviderRunner struct {
	mu             sync.Mutex
	calls          int
	completed      int
	active         int
	peak           int
	started        chan int
	releaseBlocked chan struct{}
}

func newLiveCapacityJavaScriptProviderRunner() *liveCapacityJavaScriptProviderRunner {
	return &liveCapacityJavaScriptProviderRunner{
		started:        make(chan int, 8),
		releaseBlocked: make(chan struct{}),
	}
}

func (p *liveCapacityJavaScriptProviderRunner) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	p.mu.Unlock()
	p.started <- call
	defer func() {
		p.mu.Lock()
		p.active--
		p.completed++
		p.mu.Unlock()
	}()
	if call <= 2 {
		select {
		case <-p.releaseBlocked:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	label := "javascript-child-one"
	if strings.Contains(string(request.Stdin), "javascript capacity child two") {
		label = "javascript-child-two"
	}
	return platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(fmt.Sprintf(`{"text":"live capacity complete","label":%q}`, label)),
	}, nil
}

func (p *liveCapacityJavaScriptProviderRunner) waitForCall(t *testing.T, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), liveCapacityTestTimeout)
	defer cancel()
	for {
		select {
		case call := <-p.started:
			if call >= want {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for JavaScript provider call %d", want)
		}
	}
}

type liveCapacityJavaScriptProviderState struct {
	calls     int
	completed int
	active    int
	peak      int
}

func (p *liveCapacityJavaScriptProviderRunner) snapshot() liveCapacityJavaScriptProviderState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return liveCapacityJavaScriptProviderState{
		calls:     p.calls,
		completed: p.completed,
		active:    p.active,
		peak:      p.peak,
	}
}

var _ platformprocess.CommandRunner = (*liveCapacityJavaScriptProviderRunner)(nil)
