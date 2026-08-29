package composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const parallelConcurrentDispatchWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "summarize alpha", label: "child-alpha" },
    { prompt: "summarize beta", label: "child-beta" },
  ]);
  return { results };
})();`

const parallelDeclaredResultOrderingWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "child-alpha", label: "child-alpha" },
    { prompt: "child-beta", label: "child-beta" },
    { prompt: "child-gamma", label: "child-gamma" },
  ]);
  return { results };
})();`

var parallelDeclaredResultOrderingLabels = []string{
	"child-alpha",
	"child-beta",
	"child-gamma",
}

const parallelPartialFailureWorkflow = `return (async function () {
  const results = await parallel([
    { prompt: "child-alpha", label: "child-alpha" },
    { prompt: "child-beta and force provider failure", label: "child-beta" },
    { prompt: "child-gamma", label: "child-gamma" },
  ]);
  return { results };
})();`

// TestJavaScriptParallelDispatchesChildrenConcurrently proves JavaScript parallel
// keeps more than one external child call in flight at the same time through the
// public Factory Session and dispatch surfaces, using controllable provider edges
// instead of wall-clock sleeps to observe concurrency.
func runJavaScriptParallelDispatchesChildrenConcurrently(t *testing.T, fixture *compositionFixture) {
	provider := newGatedParallelChildProvider()
	registerCompositionProvider(t, fixture, provider)

	started := startParallelCompositionWorkflowAsync(
		t,
		fixture.baseURL,
		"parallel-composition-concurrent-dispatch",
		parallelConcurrentDispatchWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)

	provider.waitForConcurrentCalls(t, 5*time.Second)
	provider.releaseAll()

	completed := waitForParallelCompositionSessionStatus(
		t,
		fixture.baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		10*time.Second,
	)
	if completed.ResultSummary == nil ||
		completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", completed.ResultSummary)
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		fixture.baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2 public child dispatches", len(dispatches.Dispatches))
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
	}
	if provider.peakActive() < 2 {
		t.Fatalf("provider peak active child calls = %d, want at least 2 concurrent external calls", provider.peakActive())
	}
}

// TestJavaScriptParallelPreservesDeclaredResultOrdering proves JavaScript parallel
// returns child results in declared input order on the public Factory Session result
// surface even when controllable external edges complete children in a different order.
func runJavaScriptParallelPreservesDeclaredResultOrdering(t *testing.T, fixture *compositionFixture) {
	provider := newLabelGatedParallelChildProvider(parallelDeclaredResultOrderingLabels)
	registerCompositionProvider(t, fixture, provider)

	started := startParallelCompositionWorkflowAsync(
		t,
		fixture.baseURL,
		"parallel-composition-declared-ordering",
		parallelDeclaredResultOrderingWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)

	waitForParallelCompositionInFlightDispatches(t, fixture.baseURL, sessionID, 3, 5*time.Second)
	for _, label := range []string{"child-gamma", "child-beta", "child-alpha"} {
		provider.releaseLabel(label)
		waitForParallelCompositionLabelCompletion(t, provider, label, 5*time.Second)
	}

	completed := waitForParallelCompositionSessionStatus(
		t,
		fixture.baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		10*time.Second,
	)
	if completed.ResultSummary == nil ||
		completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", completed.ResultSummary)
	}

	wantCompletionOrder := []string{"child-gamma", "child-beta", "child-alpha"}
	if got := provider.completionOrder(); !reflect.DeepEqual(got, wantCompletionOrder) {
		t.Fatalf("provider completion order = %v, want %v", got, wantCompletionOrder)
	}

	resultPayload := readParallelCompositionFinalResult(t, fixture.baseURL, sessionID)
	assertParallelCompositionResultLabels(t, resultPayload, parallelDeclaredResultOrderingLabels)

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		fixture.baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 3 public child dispatches", len(dispatches.Dispatches))
	}
	assertParallelCompositionDispatchLabels(t, dispatches.Dispatches, parallelDeclaredResultOrderingLabels)
}

// TestJavaScriptParallelPartialFailureUsesDocumentedPolicy proves JavaScript parallel
// surfaces one failed child as an explicit failed result with a stable diagnostic while
// successful siblings still complete, matching the documented partial-failure policy rather
// than aborting the whole workflow call.
func runJavaScriptParallelPartialFailureUsesDocumentedPolicy(t *testing.T, fixture *compositionFixture) {
	provider := newPartialFailureParallelChildProvider()
	registerCompositionProvider(t, fixture, provider)

	started := startParallelCompositionWorkflowAsync(
		t,
		fixture.baseURL,
		"parallel-composition-partial-failure",
		parallelPartialFailureWorkflow,
	)
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	fixture.trackLiveSession(t, sessionID)

	completed := waitForParallelCompositionSessionStatus(
		t,
		fixture.baseURL,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		15*time.Second,
	)
	if completed.ResultSummary == nil ||
		completed.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", completed.ResultSummary)
	}
	if completed.Progress == nil ||
		intValueOrZero(completed.Progress.TotalDispatches) != 3 ||
		intValueOrZero(completed.Progress.CompletedDispatches) != 2 ||
		intValueOrZero(completed.Progress.FailedDispatches) != 1 {
		t.Fatalf("progress = %#v, want three dispatches with two completed and one failed", completed.Progress)
	}

	resultPayload := readParallelCompositionFinalResult(t, fixture.baseURL, sessionID)
	assertParallelCompositionPartialFailureResults(t, resultPayload, parallelDeclaredResultOrderingLabels)

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		fixture.baseURL+"/factory-sessions/"+sessionID+"/dispatches",
	)
	if len(dispatches.Dispatches) != 3 {
		t.Fatalf("dispatch count = %d, want 3 public child dispatches", len(dispatches.Dispatches))
	}
	assertParallelCompositionPartialFailureDispatches(t, dispatches.Dispatches, parallelDeclaredResultOrderingLabels)
}

func parallelCompositionFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":      "process",
				"worker":    "worker-a",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

func writeParallelCompositionGlobalConfig(t *testing.T, homeDir string) {
	t.Helper()
	configDir := filepath.Join(homeDir, ".you-agent-factory")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir global config directory: %v", err)
	}
	config := []byte(`{
  "defaults": {"workerModelProvider": "openai", "workerModel": "default-model"}
}`)
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), config, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
}

func startParallelCompositionWorkflowAsync(
	t *testing.T,
	baseURL, requestID, workflowSource string,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()

	dialect := "you-workflow-v1"
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: requestID,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
			InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
				Dialect: &dialect,
				InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
					Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
					Inline:   workflowSource,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal parallel workflow request: %v", err)
	}

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		baseURL+"/factory-sessions/async",
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("build async parallel workflow request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("start async parallel workflow: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("start async parallel workflow status = %d: %s", response.StatusCode, body.String())
	}

	var started factoryapi.FactorySessionExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&started); err != nil {
		t.Fatalf("decode async parallel workflow response: %v", err)
	}
	return started
}

func waitForParallelCompositionInFlightDispatches(
	t *testing.T,
	baseURL, sessionID string,
	want int,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readParallelCompositionSession(t, baseURL, sessionID)
		if session.Progress != nil &&
			intValueOrZero(session.Progress.InFlightDispatches) >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := readParallelCompositionSession(t, baseURL, sessionID)
	t.Fatalf(
		"session %s inFlightDispatches = %#v, want at least %d within %s",
		sessionID,
		session.Progress,
		want,
		timeout,
	)
}

func waitForParallelCompositionSessionStatus(
	t *testing.T,
	baseURL, sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readParallelCompositionSession(t, baseURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(10 * time.Millisecond)
	}
	session := readParallelCompositionSession(t, baseURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func readParallelCompositionSession(
	t *testing.T,
	baseURL, sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()
	return support.GetJSON[factoryapi.FactorySessionDurableReadModel](
		t,
		baseURL+"/factory-sessions/"+sessionID,
	)
}

func intValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func readParallelCompositionFinalResult(
	t *testing.T,
	baseURL, sessionID string,
) map[string]any {
	t.Helper()

	result := support.GetJSON[factoryapi.FactorySessionResult](
		t,
		baseURL+"/factory-sessions/"+sessionID+"/results?mode=final",
	)
	if result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
	if result.PrimaryResult == nil || len(*result.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one content part", result.PrimaryResult)
	}
	part, err := (*result.PrimaryResult)[0].AsWorkJsonContentPart()
	if err != nil {
		t.Fatalf("decode primary result json part: %v", err)
	}
	payload, ok := part.Json.(map[string]any)
	if !ok {
		t.Fatalf("primary json payload = %#v, want object", part.Json)
	}
	return payload
}

func assertParallelCompositionResultLabels(
	t *testing.T,
	payload map[string]any,
	wantLabels []string,
) {
	t.Helper()

	results, ok := payload["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("result.results = %#v, want %d entries", payload["results"], len(wantLabels))
	}
	for index, wantLabel := range wantLabels {
		child, ok := results[index].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object child result", index, results[index])
		}
		gotLabel, _ := child["label"].(string)
		gotStatus, _ := child["status"].(string)
		if gotLabel != wantLabel || gotStatus != "COMPLETED" {
			t.Fatalf(
				"results[%d] = label=%q status=%q, want label=%q status=COMPLETED",
				index,
				gotLabel,
				gotStatus,
				wantLabel,
			)
		}
	}
}

func assertParallelCompositionPartialFailureResults(
	t *testing.T,
	payload map[string]any,
	wantLabels []string,
) {
	t.Helper()

	results, ok := payload["results"].([]any)
	if !ok || len(results) != len(wantLabels) {
		t.Fatalf("result.results = %#v, want %d entries", payload["results"], len(wantLabels))
	}
	for index, wantLabel := range wantLabels {
		child, ok := results[index].(map[string]any)
		if !ok {
			t.Fatalf("results[%d] = %#v, want object child result", index, results[index])
		}
		gotLabel, _ := child["label"].(string)
		if gotLabel != wantLabel {
			t.Fatalf("results[%d].label = %q, want %q", index, gotLabel, wantLabel)
		}
		if wantLabel == "child-beta" {
			gotStatus, _ := child["status"].(string)
			if gotStatus != "FAILED" {
				t.Fatalf("results[%d].status = %q, want FAILED", index, gotStatus)
			}
			diagnostic, _ := child["diagnostic"].(string)
			if strings.TrimSpace(diagnostic) == "" {
				t.Fatalf("results[%d].diagnostic is empty, want non-empty public diagnostic", index)
			}
			if child["artifactRef"] != nil {
				t.Fatalf("results[%d].artifactRef = %#v, want absent on failed child", index, child["artifactRef"])
			}
			continue
		}
		gotStatus, _ := child["status"].(string)
		if gotStatus != "COMPLETED" {
			t.Fatalf("results[%d].status = %q, want COMPLETED", index, gotStatus)
		}
	}
}

func assertParallelCompositionPartialFailureDispatches(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	wantLabels []string,
) {
	t.Helper()

	gotByLabel := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		gotByLabel[*dispatch.Label] = dispatch
	}
	for _, wantLabel := range wantLabels {
		dispatch, ok := gotByLabel[wantLabel]
		if !ok {
			t.Fatalf("dispatch labels = %v, missing %q", gotByLabel, wantLabel)
		}
		if wantLabel == "child-beta" {
			if dispatch.Status != factoryapi.FactoryDispatchStatusFAILED {
				t.Fatalf("dispatch %s status = %q, want FAILED", dispatch.Id, dispatch.Status)
			}
			if dispatch.FailureDetail == nil || strings.TrimSpace(dispatch.FailureDetail.Message) == "" {
				t.Fatalf("dispatch %s failureDetail = %#v, want non-empty public diagnostic", dispatch.Id, dispatch.FailureDetail)
			}
			if dispatch.OutputArtifactIds != nil && len(*dispatch.OutputArtifactIds) > 0 {
				t.Fatalf("dispatch %s outputArtifactIds = %#v, want none on failed child", dispatch.Id, dispatch.OutputArtifactIds)
			}
			continue
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		if dispatch.FailureDetail != nil {
			t.Fatalf("dispatch %s failureDetail = %#v, want nil on successful sibling", dispatch.Id, dispatch.FailureDetail)
		}
	}
}

func assertParallelCompositionDispatchLabels(
	t *testing.T,
	dispatches []factoryapi.FactorySessionDispatchSummary,
	wantLabels []string,
) {
	t.Helper()

	gotLabels := make(map[string]factoryapi.FactorySessionDispatchSummary, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Label == nil || strings.TrimSpace(*dispatch.Label) == "" {
			t.Fatalf("dispatch %s missing label, want labeled child dispatches", dispatch.Id)
		}
		if dispatch.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
			t.Fatalf("dispatch %s status = %q, want COMPLETED", dispatch.Id, dispatch.Status)
		}
		gotLabels[*dispatch.Label] = dispatch
	}
	for _, wantLabel := range wantLabels {
		if _, ok := gotLabels[wantLabel]; !ok {
			t.Fatalf("dispatch labels = %v, missing %q", gotLabels, wantLabel)
		}
	}
}

func waitForParallelCompositionLabelCompletion(
	t *testing.T,
	provider *labelGatedParallelChildProvider,
	label string,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if provider.hasCompleted(label) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("label %q did not complete within %s; completion order = %v", label, timeout, provider.completionOrder())
}

type gatedParallelChildProvider struct {
	testutil.NativeProvider
	mu                  sync.Mutex
	active              int
	peak                int
	release             chan struct{}
	concurrent          chan struct{}
	releaseOnce         sync.Once
	concurrentCallsOnce sync.Once
}

func newGatedParallelChildProvider() *gatedParallelChildProvider {
	provider := &gatedParallelChildProvider{
		release:    make(chan struct{}),
		concurrent: make(chan struct{}),
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (p *gatedParallelChildProvider) waitForConcurrentCalls(t *testing.T, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.concurrent:
	case <-timer.C:
		t.Fatalf("provider did not observe two concurrent child calls within %s; peak active calls = %d", timeout, p.peakActive())
	}
}

func (p *gatedParallelChildProvider) releaseAll() {
	p.releaseOnce.Do(func() {
		close(p.release)
	})
}

func (p *gatedParallelChildProvider) peakActive() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

func (p *gatedParallelChildProvider) Execute(
	ctx context.Context,
	req providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	p.mu.Lock()
	p.active++
	if p.active > p.peak {
		p.peak = p.active
	}
	if p.active >= 2 {
		p.concurrentCallsOnce.Do(func() { close(p.concurrent) })
	}
	p.mu.Unlock()

	select {
	case <-p.release:
	case <-ctx.Done():
		p.mu.Lock()
		p.active--
		p.mu.Unlock()
		return providers.ExecuteResult{}, ctx.Err()
	}

	p.mu.Lock()
	p.active--
	p.mu.Unlock()

	label := parallelChildLabelFromRequest(req)
	return providers.ExecuteResult{
		Content: fmt.Sprintf(`{"text":"parallel-child:%s:COMPLETE","label":%q}`, label, label),
	}, nil
}

func parallelChildLabelFromRequest(req providers.ExecuteRequest) string {
	for _, token := range req.InputTokens {
		payload, ok := token.(map[string]any)
		if !ok {
			continue
		}
		color, ok := payload["color"].(map[string]any)
		if !ok {
			continue
		}
		tags, ok := color["tags"].(map[string]any)
		if !ok {
			continue
		}
		if label, ok := tags["label"].(string); ok && label != "" {
			return label
		}
	}
	message := strings.TrimSpace(req.UserMessage)
	if message == "" {
		return "child"
	}
	return message
}

type labelGatedParallelChildProvider struct {
	testutil.NativeProvider
	mu              sync.Mutex
	gates           map[string]chan struct{}
	releaseOnce     map[string]*sync.Once
	completedLabels []string
}

func newLabelGatedParallelChildProvider(labels []string) *labelGatedParallelChildProvider {
	provider := &labelGatedParallelChildProvider{
		gates:       make(map[string]chan struct{}, len(labels)),
		releaseOnce: make(map[string]*sync.Once, len(labels)),
	}
	for _, label := range labels {
		provider.gates[label] = make(chan struct{})
		provider.releaseOnce[label] = &sync.Once{}
	}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (p *labelGatedParallelChildProvider) releaseLabel(label string) {
	once, ok := p.releaseOnce[label]
	if !ok {
		return
	}
	once.Do(func() {
		close(p.gates[label])
	})
}

func (p *labelGatedParallelChildProvider) hasCompleted(label string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, completed := range p.completedLabels {
		if completed == label {
			return true
		}
	}
	return false
}

func (p *labelGatedParallelChildProvider) completionOrder() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.completedLabels...)
}

func (p *labelGatedParallelChildProvider) Execute(
	ctx context.Context,
	req providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	label := parallelChildLabelFromRequest(req)
	gate, ok := p.gates[label]
	if !ok {
		return providers.ExecuteResult{}, fmt.Errorf("unexpected parallel child label %q", label)
	}

	select {
	case <-gate:
	case <-ctx.Done():
		return providers.ExecuteResult{}, ctx.Err()
	}

	p.mu.Lock()
	p.completedLabels = append(p.completedLabels, label)
	p.mu.Unlock()

	return providers.ExecuteResult{
		Content: fmt.Sprintf(`{"text":"parallel-child:%s:COMPLETE","label":%q}`, label, label),
	}, nil
}

type partialFailureParallelChildProvider struct {
	testutil.NativeProvider
}

func newPartialFailureParallelChildProvider() *partialFailureParallelChildProvider {
	provider := &partialFailureParallelChildProvider{}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (p *partialFailureParallelChildProvider) Execute(
	_ context.Context,
	req providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if strings.Contains(req.UserMessage, "force provider failure") {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindInvalidRequest,
			Message: "Provider rejected the request as invalid.",
		}
	}

	label := parallelChildLabelFromRequest(req)
	return providers.ExecuteResult{
		Content: fmt.Sprintf(`{"text":"parallel-child:%s:COMPLETE","label":%q}`, label, label),
	}, nil
}
