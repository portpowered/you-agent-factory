package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const concurrentSessionWorkersTimeout = 15 * time.Second

// TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage
// proves the process-scoped Workers service can execute overlapping requests
// for two public Factory Sessions without borrowing the other session's prompt
// or identity. Closing the first session cancels its provider attempt, while
// the second invocation completes and the same service accepts a later request
// from that surviving session.
func TestFactoryRuntimeConcurrentSessionsShareWorkersWithoutCancellationLeakage(t *testing.T) {
	t.Parallel()

	dir := support.ScaffoldFactory(t, concurrentSessionWorkersFactoryConfig())
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))

	provider := newConcurrentSessionWorkersProvider()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderOverride: provider},
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	defaultSession := support.GetDefaultSession(t, baseURL)
	if defaultSession.Id == "" {
		t.Fatalf("default Factory Session = %#v, want session identity", defaultSession)
	}
	openedSession := support.OpenFactorySessionAt(t, baseURL, dir)
	if openedSession.Session == nil || openedSession.Session.Id == "" {
		t.Fatalf("opened Factory Session = %#v, want session identity", openedSession)
	}
	openedSessionID := openedSession.Session.Id
	var openedSessionClosed atomic.Bool
	t.Cleanup(func() {
		if !openedSessionClosed.Load() {
			support.CloseFactorySessionAt(t, baseURL, openedSessionID)
		}
	})

	const (
		cancelledPrompt = "shared-workers-cancelled-prompt"
		survivingPrompt = "shared-workers-surviving-prompt"
		laterPrompt     = "shared-workers-later-prompt"
	)
	cancelledSessionID := openedSessionID
	survivingSessionID := factorysessions.DefaultSessionID
	provider.configure(cancelledSessionID)

	cancelledDone := make(chan concurrentInvocationResult, 1)
	go func() {
		response, err := postConcurrentSessionInvocation(
			t.Context(),
			baseURL,
			cancelledSessionID,
			cancelledPrompt,
		)
		cancelledDone <- concurrentInvocationResult{response: response, err: err}
	}()
	survivingDone := make(chan concurrentInvocationResult, 1)
	go func() {
		response, err := postConcurrentSessionInvocation(
			t.Context(),
			baseURL,
			survivingSessionID,
			survivingPrompt,
		)
		survivingDone <- concurrentInvocationResult{response: response, err: err}
	}()

	requests := map[string]workers.ProviderInferenceRequest{}
	for range []string{cancelledSessionID, survivingSessionID} {
		request := provider.waitStartedAny(t)
		requests[request.Correlation.FactorySessionID] = request
	}
	cancelledRequest, ok := requests[cancelledSessionID]
	if !ok {
		t.Fatalf("provider requests did not include cancelled session %q: %#v", cancelledSessionID, requests)
	}
	survivingRequest, ok := requests[survivingSessionID]
	if !ok {
		t.Fatalf("provider requests did not include surviving session %q: %#v", survivingSessionID, requests)
	}

	assertConcurrentSessionRequest(t, cancelledRequest, cancelledSessionID, cancelledPrompt)
	assertConcurrentSessionRequest(t, survivingRequest, survivingSessionID, survivingPrompt)
	assertDistinctConcurrentAttemptIdentity(t, cancelledRequest, survivingRequest)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- closeConcurrentSession(baseURL, cancelledSessionID)
	}()
	provider.waitCancelled(t, cancelledSessionID)
	if err := <-closeDone; err != nil {
		t.Fatalf("close cancelled Factory Session: %v", err)
	}
	openedSessionClosed.Store(true)
	provider.releaseSurvivingAttempt()

	cancelledResult := awaitConcurrentInvocation(t, cancelledDone)
	if cancelledResult.err != nil &&
		!errors.Is(cancelledResult.err, context.Canceled) &&
		!strings.Contains(cancelledResult.err.Error(), "status = 404") {
		t.Fatalf("cancelled invocation error = %v", cancelledResult.err)
	}
	if cancelledResult.err == nil &&
		cancelledResult.response.Status != factoryapi.InvocationTerminalStatusCanceled {
		t.Fatalf("cancelled invocation response = %#v, want CANCELED", cancelledResult.response)
	}

	survivingResult := awaitConcurrentInvocation(t, survivingDone)
	if survivingResult.err != nil {
		t.Fatalf("surviving invocation error = %v", survivingResult.err)
	}
	if survivingResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("surviving invocation response = %#v, want COMPLETED", survivingResult.response)
	}
	assertConcurrentInvocationPrimaryResult(t, survivingResult.response, survivingSessionID)

	laterDone := make(chan concurrentInvocationResult, 1)
	go func() {
		response, err := postConcurrentSessionInvocation(
			t.Context(),
			baseURL,
			survivingSessionID,
			laterPrompt,
		)
		laterDone <- concurrentInvocationResult{response: response, err: err}
	}()
	laterRequest := provider.waitStartedAny(t)
	assertConcurrentSessionRequest(t, laterRequest, survivingSessionID, laterPrompt)
	laterResult := awaitConcurrentInvocation(t, laterDone)
	if laterResult.err != nil {
		t.Fatalf("later invocation error = %v", laterResult.err)
	}
	if laterResult.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("later invocation response = %#v, want COMPLETED", laterResult.response)
	}
	assertConcurrentInvocationPrimaryResult(t, laterResult.response, survivingSessionID)

	calls := provider.requests()
	if len(calls) != 3 {
		t.Fatalf("provider requests = %d, want cancelled, surviving, and later attempts: %#v", len(calls), calls)
	}
	if calls[2].Correlation.FactorySessionID != survivingSessionID {
		t.Fatalf("later provider request session = %q, want %q", calls[2].Correlation.FactorySessionID, survivingSessionID)
	}
	if provider.activeCount() != 0 {
		t.Fatalf("provider active attempts = %d after terminal outcomes, want 0", provider.activeCount())
	}
}

type concurrentInvocationResult struct {
	response factoryapi.InvocationResponse
	err      error
}

func closeConcurrentSession(baseURL string, sessionID string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	request, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return fmt.Errorf("DELETE %s status = %d: read response body: %w", endpoint, response.StatusCode, readErr)
		}
		return fmt.Errorf("DELETE %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func postConcurrentSessionInvocation(
	ctx context.Context,
	baseURL string,
	sessionID string,
	prompt string,
) (factoryapi.InvocationResponse, error) {
	invocation, err := concurrentSessionInvocationRequest(prompt)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	payload, err := json.Marshal(invocation)
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
		return factoryapi.InvocationResponse{}, fmt.Errorf(
			"POST %s status = %d: %s",
			endpoint,
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

func concurrentSessionInvocationRequest(text string) (factoryapi.InvocationRequest, error) {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		return factoryapi.InvocationRequest{}, err
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}, nil
}

func awaitConcurrentInvocation(
	t *testing.T,
	results <-chan concurrentInvocationResult,
) concurrentInvocationResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(concurrentSessionWorkersTimeout):
		t.Fatal("timed out waiting for Factory Session invocation")
		return concurrentInvocationResult{}
	}
}

func concurrentSessionWorkersFactoryConfig() map[string]any {
	return map[string]any{
		"name": "shared-workers",
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
	}
}

func assertConcurrentSessionRequest(
	t *testing.T,
	request workers.ProviderInferenceRequest,
	wantSessionID string,
	wantPromptMarker string,
) {
	t.Helper()
	correlation := request.Correlation
	for name, value := range map[string]string{
		"factory session": correlation.FactorySessionID,
		"runtime":         correlation.RuntimeID,
		"generation":      correlation.GenerationID,
		"dispatch":        correlation.DispatchID,
		"attempt":         correlation.AttemptID,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("%s correlation is empty in provider request: %#v", name, request)
		}
	}
	if correlation.FactorySessionID != wantSessionID {
		t.Fatalf("provider request session = %q, want %q", correlation.FactorySessionID, wantSessionID)
	}
	if len(request.Dispatch.Execution.WorkIDs) == 0 {
		t.Fatalf("provider request Work IDs are empty: %#v", request.Dispatch.Execution)
	}
	if !providerRequestContainsPrompt(request, wantPromptMarker) {
		t.Fatalf("provider request prompt/input = %#v, want session-specific marker %q", request, wantPromptMarker)
	}
}

func providerRequestContainsPrompt(request workers.ProviderInferenceRequest, marker string) bool {
	if prompt := request.SystemPrompt + "\n" + request.UserMessage; strings.Contains(prompt, marker) {
		return true
	}
	for _, input := range request.InputTokens {
		token, ok := input.(workers.Token)
		if !ok {
			continue
		}
		for _, part := range token.Color.Content {
			if strings.Contains(part.Text, marker) {
				return true
			}
		}
	}
	return false
}

func assertDistinctConcurrentAttemptIdentity(
	t *testing.T,
	first workers.ProviderInferenceRequest,
	second workers.ProviderInferenceRequest,
) {
	t.Helper()
	if first.Correlation.FactorySessionID == second.Correlation.FactorySessionID {
		t.Fatalf("concurrent provider requests share Factory Session identity: %#v / %#v", first.Correlation, second.Correlation)
	}
	if first.Correlation.RuntimeID == second.Correlation.RuntimeID {
		t.Fatalf("concurrent provider requests share Runtime identity: %#v / %#v", first.Correlation, second.Correlation)
	}
	if first.Correlation.DispatchID == second.Correlation.DispatchID {
		t.Fatalf("concurrent provider requests share dispatch identity: %#v / %#v", first.Correlation, second.Correlation)
	}
	if first.Correlation.AttemptID == second.Correlation.AttemptID {
		t.Fatalf("concurrent provider requests share attempt identity: %#v / %#v", first.Correlation, second.Correlation)
	}
}

func assertConcurrentInvocationPrimaryResult(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantSessionID string,
) {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primary result = %#v, want one text part for session %q", response.PrimaryResult, wantSessionID)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("invocation primary result part = %#v: %v", (*response.PrimaryResult)[0], err)
	}
	if part.Text != wantSessionID {
		t.Fatalf("invocation primary result text = %q, want provider output for session %q", part.Text, wantSessionID)
	}
}

type concurrentSessionWorkersProvider struct {
	mu sync.Mutex

	cancelSessionID string
	cancelled       []workers.ProviderInferenceRequest
	seenRequests    []workers.ProviderInferenceRequest
	active          atomic.Int32
	holding         atomic.Bool

	started         chan workers.ProviderInferenceRequest
	cancelledSignal chan workers.ProviderInferenceRequest
	release         chan struct{}
	releaseOnce     sync.Once
}

func newConcurrentSessionWorkersProvider() *concurrentSessionWorkersProvider {
	provider := &concurrentSessionWorkersProvider{
		started:         make(chan workers.ProviderInferenceRequest, 8),
		cancelledSignal: make(chan workers.ProviderInferenceRequest, 4),
		release:         make(chan struct{}),
	}
	provider.holding.Store(true)
	return provider
}

func (p *concurrentSessionWorkersProvider) configure(cancelSessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelSessionID = cancelSessionID
}

func (p *concurrentSessionWorkersProvider) Infer(
	ctx context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	request = workers.CloneProviderInferenceRequest(request)
	p.active.Add(1)
	defer p.active.Add(-1)

	p.mu.Lock()
	p.seenRequests = append(p.seenRequests, request)
	p.mu.Unlock()

	select {
	case p.started <- request:
	case <-ctx.Done():
		return workers.InferenceResponse{}, ctx.Err()
	}

	if !p.holding.Load() {
		return p.successResponse(request), nil
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		p.mu.Lock()
		cancelSessionID := p.cancelSessionID
		p.mu.Unlock()
		if request.Correlation.FactorySessionID != cancelSessionID {
			return workers.InferenceResponse{}, fmt.Errorf(
				"unexpected cancellation for Factory Session %q, want %q",
				request.Correlation.FactorySessionID,
				cancelSessionID,
			)
		}
		p.mu.Lock()
		p.cancelled = append(p.cancelled, request)
		p.mu.Unlock()
		select {
		case p.cancelledSignal <- request:
		default:
		}
		return workers.InferenceResponse{}, ctx.Err()
	}

	return p.successResponse(request), nil
}

func (p *concurrentSessionWorkersProvider) successResponse(
	request workers.ProviderInferenceRequest,
) workers.InferenceResponse {
	sessionID := request.Correlation.FactorySessionID
	return workers.InferenceResponse{
		Content: sessionID,
		// Provider overrides own the normalized outcome when they bypass the
		// native runner. Mark this controlled response as accepted so the
		// test exercises concurrent session isolation rather than stop-token
		// classification.
		Outcome: workers.OutcomeAccepted,
		Diagnostics: &workers.WorkDiagnostics{Metadata: map[string]string{
			"test_factory_session_id": sessionID,
		}},
	}
}

func (p *concurrentSessionWorkersProvider) waitStartedAny(t *testing.T) workers.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.started:
		return request
	case <-time.After(concurrentSessionWorkersTimeout):
		t.Fatal("provider did not observe a Factory Session entering execution")
		return workers.ProviderInferenceRequest{}
	}
}

func (p *concurrentSessionWorkersProvider) waitCancelled(t *testing.T, wantSessionID string) workers.ProviderInferenceRequest {
	t.Helper()
	select {
	case request := <-p.cancelledSignal:
		if request.Correlation.FactorySessionID != wantSessionID {
			t.Fatalf("provider cancelled session = %q, want %q", request.Correlation.FactorySessionID, wantSessionID)
		}
		return request
	case <-time.After(concurrentSessionWorkersTimeout):
		t.Fatalf("provider did not observe Factory Session %q cancellation", wantSessionID)
		return workers.ProviderInferenceRequest{}
	}
}

func (p *concurrentSessionWorkersProvider) releaseSurvivingAttempt() {
	p.releaseOnce.Do(func() {
		p.holding.Store(false)
		close(p.release)
	})
}

func (p *concurrentSessionWorkersProvider) requests() []workers.ProviderInferenceRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	requests := make([]workers.ProviderInferenceRequest, len(p.seenRequests))
	for index, request := range p.seenRequests {
		requests[index] = workers.CloneProviderInferenceRequest(request)
	}
	return requests
}

func (p *concurrentSessionWorkersProvider) activeCount() int32 {
	return p.active.Load()
}

var _ workers.Provider = (*concurrentSessionWorkersProvider)(nil)
