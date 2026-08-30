package agy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessionshttp "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/http"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func openAgyFactorySessionAt(
	t testing.TB,
	baseURL string,
	folderPath string,
	targetName string,
) factoryapi.OpenFactorySessionResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.OpenFactorySessionRequest{
		FolderPath: folderPath,
		Target: &factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: &targetName,
		},
	})
	if err != nil {
		t.Fatalf("marshal open AGY Factory Session request: %v", err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.NewDecoder(response.Body).Decode(&opened); err != nil {
		t.Fatalf("decode POST %s: %v", endpoint, err)
	}
	return opened
}

func waitForAgyRuntimeReady(baseURL string, timeout time.Duration) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/status"
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// Process startup is asynchronous and this observer proves the public HTTP
	// readiness boundary, so a controlled command result cannot replace it.
	poll := time.NewTicker(10 * time.Millisecond)
	defer poll.Stop()
	var lastErr error
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			closeErr := response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && closeErr == nil && strings.TrimSpace(status.RuntimeStatus) != "" {
				return nil
			}
			lastErr = fmt.Errorf("status=%d decode=%v close=%v", response.StatusCode, decodeErr, closeErr)
		} else {
			lastErr = err
		}
		select {
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for AGY runtime readiness at %s: %w", endpoint, lastErr)
		case <-poll.C:
		}
	}
}

func waitForAgySessionFactoryEvents(
	ctx context.Context,
	baseURL string,
	sessionID string,
	want int,
	timeout time.Duration,
) ([]factoryapi.FactoryEvent, error) {
	if want <= 0 {
		return nil, fmt.Errorf("AGY Factory Event target count = %d, want positive count", want)
	}
	observeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	endpoint := support.SessionEventsURL(baseURL, sessionID)
	request, err := http.NewRequestWithContext(observeContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	// Keep one public retained-plus-live SSE subscription open. Reopening the
	// endpoint every 10 ms rebuilt a subscription and decoded the retained
	// ledger on every attempt, while this observer provides the same exact
	// event-count barrier without repeated snapshot work.
	eventsCh := make(chan factoryapi.FactoryEvent, want)
	streamDone := make(chan error, 1)
	go func() {
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			streamDone <- err
			return
		}

		var streamErr error
		if response.StatusCode != http.StatusOK {
			body, readErr := io.ReadAll(response.Body)
			streamErr = errors.Join(readErr, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body))))
		} else {
			retainedCount, parseErr := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessionshttp.SessionEventStreamRetainedCountHeader)))
			switch {
			case parseErr != nil:
				streamErr = fmt.Errorf("GET %s retained event count: %w", endpoint, parseErr)
			case retainedCount > want:
				streamErr = fmt.Errorf("GET %s retained event count = %d, want at most %d", endpoint, retainedCount, want)
			default:
				scanner := bufio.NewScanner(response.Body)
			readLoop:
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if !strings.HasPrefix(line, "data:") {
						continue
					}
					var event factoryapi.FactoryEvent
					if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
						streamErr = fmt.Errorf("decode factory event: %w", err)
						break
					}
					select {
					case eventsCh <- event:
					case <-observeContext.Done():
						streamErr = observeContext.Err()
						break readLoop
					}
				}
				if streamErr == nil {
					streamErr = scanner.Err()
				}
			}
		}
		streamErr = errors.Join(streamErr, response.Body.Close())
		streamDone <- streamErr
	}()

	events := make([]factoryapi.FactoryEvent, 0, want)
	for len(events) < want {
		select {
		case event := <-eventsCh:
			events = append(events, event)
		case streamErr := <-streamDone:
			if observeContext.Err() != nil {
				return nil, fmt.Errorf("Factory Session %q Factory Events: got %d, want %d: %w", sessionID, len(events), want, observeContext.Err())
			}
			return nil, fmt.Errorf("Factory Session %q Factory Events: got %d, want %d: %v", sessionID, len(events), want, streamErr)
		case <-observeContext.Done():
			streamErr := <-streamDone
			return nil, fmt.Errorf("Factory Session %q Factory Events: got %d, want %d: %v", sessionID, len(events), want, streamErr)
		}
	}

	cancel()
	if streamErr := <-streamDone; streamErr != nil && !errors.Is(streamErr, context.Canceled) && !errors.Is(streamErr, context.DeadlineExceeded) {
		return nil, fmt.Errorf("Factory Session %q Factory Events stream after %d events: %w", sessionID, len(events), streamErr)
	}
	return events, nil
}

// readAgyFactoryEventsAfterResponse uses the cheapest public observation that
// can prove the expected terminal ledger. A retained snapshot is complete in
// the usual case after all response frames have arrived; only a publication
// race opens the retained-plus-live fallback stream.
func readAgyFactoryEventsAfterResponse(
	ctx context.Context,
	baseURL string,
	sessionID string,
	want int,
	timeout time.Duration,
) ([]factoryapi.FactoryEvent, error) {
	if want <= 0 {
		return nil, fmt.Errorf("AGY Factory Event target count = %d, want positive count", want)
	}
	observeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	events, complete, err := readAgyFactoryEventSnapshot(observeContext, baseURL, sessionID, want)
	if err != nil {
		return nil, err
	}
	if complete {
		return events, nil
	}
	return waitForAgySessionFactoryEvents(observeContext, baseURL, sessionID, want, timeout)
}

func readAgyFactoryEventSnapshot(
	ctx context.Context,
	baseURL string,
	sessionID string,
	want int,
) (events []factoryapi.FactoryEvent, complete bool, err error) {
	endpoint := support.SessionEventsURL(baseURL, sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, false, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()
	if response.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(response.Body)
		return nil, false, errors.Join(readErr, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body))))
	}
	retainedCount, err := strconv.Atoi(strings.TrimSpace(response.Header.Get(factorysessionshttp.SessionEventStreamRetainedCountHeader)))
	if err != nil {
		return nil, false, fmt.Errorf("GET %s retained event count: %w", endpoint, err)
	}
	if retainedCount > want {
		return nil, false, fmt.Errorf("GET %s retained event count = %d, want at most %d", endpoint, retainedCount, want)
	}
	if retainedCount != want {
		// The retained-plus-live fallback will decode this history once. Do not
		// decode a partial snapshot that the fallback would immediately replay.
		return nil, false, nil
	}
	events = make([]factoryapi.FactoryEvent, 0, retainedCount)
	scanner := bufio.NewScanner(response.Body)
	for len(events) < retainedCount && scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
			return nil, false, fmt.Errorf("decode factory event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, false, fmt.Errorf("read factory events: %w", err)
	}
	if len(events) != retainedCount {
		return nil, false, fmt.Errorf("read factory events: got %d of %d retained events", len(events), retainedCount)
	}
	return events, true, nil
}

func readAgySessionWork(ctx context.Context, baseURL, sessionID string) (factoryapi.ListWorkResponse, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return readAgyJSON[factoryapi.ListWorkResponse](ctx, endpoint)
}

func readAgyJSON[T any](ctx context.Context, endpoint string) (T, error) {
	var result T
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return result, err
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return result, errors.Join(readErr, closeErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return result, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("decode GET %s: %w", endpoint, err)
	}
	return result, nil
}

// closeAgyFactorySession first attempts the cheap public DELETE path. An
// active session still follows the public terminate/status/delete lifecycle,
// so cleanup remains valid after assertion or setup failures.
func closeAgyFactorySession(ctx context.Context, baseURL, sessionID string, terminalObserved bool) error {
	// http.Client{} uses http.DefaultTransport; keep its pool reusable across
	// the scenario's lifecycle requests instead of evicting connections.
	client := &http.Client{}
	cleanupCtx, cancel := context.WithTimeout(ctx, agySharedScenarioTimeout)
	defer cancel()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	if terminalObserved {
		// The exact terminal Factory Event count has been observed, so try the
		// cheap public DELETE before sending a redundant terminate control
		// request. A conflict still falls through to the active-session
		// termination path below if the state changed between observations.
		deleteStatus, deleteBody, err := requestAgyFactorySession(cleanupCtx, client, http.MethodDelete, endpoint)
		if err != nil {
			return err
		}
		if deleteStatus == http.StatusNoContent || deleteStatus == http.StatusNotFound {
			return nil
		}
		if deleteStatus != http.StatusConflict {
			return fmt.Errorf("delete status=%d body=%q", deleteStatus, strings.TrimSpace(string(deleteBody)))
		}
	}
	deleteStatus, deleteBody, err := requestAgyFactorySession(cleanupCtx, client, http.MethodDelete, endpoint)
	if err != nil {
		return err
	}
	if deleteStatus == http.StatusNoContent || deleteStatus == http.StatusNotFound {
		return nil
	}
	if deleteStatus != http.StatusConflict {
		return fmt.Errorf("delete status=%d body=%q", deleteStatus, strings.TrimSpace(string(deleteBody)))
	}

	terminateStatus, terminateBody, err := requestAgyFactorySession(cleanupCtx, client, http.MethodPost, endpoint+"/terminate")
	if err != nil {
		return err
	}
	terminalFromControl := false
	if terminateStatus < http.StatusOK || terminateStatus >= http.StatusMultipleChoices {
		if terminateStatus == http.StatusNotFound ||
			(terminateStatus == http.StatusConflict && strings.Contains(string(terminateBody), `"outcome":"TERMINAL_SESSION"`)) {
			// The public control response already proves terminality; avoid a
			// redundant status read before the retry DELETE.
			terminalFromControl = true
		} else {
			return fmt.Errorf("terminate status=%d body=%q", terminateStatus, strings.TrimSpace(string(terminateBody)))
		}
	}

	// Termination is asynchronous and DELETE rejects an active Factory Session.
	// This public status transition is the lifecycle boundary under test, so an
	// edge-controlled result cannot replace this bounded polling observation.
	if !terminalFromControl {
		poll := time.NewTicker(10 * time.Millisecond)
		defer poll.Stop()
		for {
			statusRequest, err := http.NewRequestWithContext(cleanupCtx, http.MethodGet, endpoint+"/status", nil)
			if err != nil {
				return err
			}
			statusResponse, err := client.Do(statusRequest)
			if err == nil {
				statusBody, bodyErr := io.ReadAll(statusResponse.Body)
				statusResponse.Body.Close()
				if bodyErr == nil && statusResponse.StatusCode == http.StatusOK {
					var status factoryapi.StatusResponse
					if json.Unmarshal(statusBody, &status) == nil &&
						(status.RuntimeStatus == "IDLE" || status.RuntimeStatus == "FINISHED") {
						break
					}
				}
			}
			select {
			case <-cleanupCtx.Done():
				return cleanupCtx.Err()
			case <-poll.C:
			}
		}
	}
	return deleteAgyFactorySession(cleanupCtx, client, endpoint)
}

func deleteAgyFactorySession(ctx context.Context, client *http.Client, endpoint string) error {
	deleteStatus, deleteBody, err := requestAgyFactorySession(ctx, client, http.MethodDelete, endpoint)
	if err != nil {
		return err
	}
	if deleteStatus != http.StatusNoContent && deleteStatus != http.StatusNotFound {
		return fmt.Errorf("delete status=%d body=%q", deleteStatus, strings.TrimSpace(string(deleteBody)))
	}
	return nil
}

func requestAgyFactorySession(
	ctx context.Context,
	client *http.Client,
	method string,
	endpoint string,
) (int, []byte, error) {
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader("{}")
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	payload, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return response.StatusCode, payload, errors.Join(readErr, closeErr)
	}
	return response.StatusCode, payload, nil
}

func writeAgyWorkerConfig(factoryDir, model string) error {
	config := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, model),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	path := filepath.Join(factoryDir, "workers", "worker", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create AGY worker config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		return fmt.Errorf("write AGY worker config: %w", err)
	}
	return nil
}

type agyFactoryAsset struct {
	relative string
	data     []byte
	mode     fs.FileMode
}

func loadAgyFactoryAssets(sourceDir string) ([]agyFactoryAsset, error) {
	// The worker file is authored after these assets are staged for each case.
	// Read the immutable topology and workstation prompt once for the package,
	// then reuse the bytes for both scenario directories.
	assets := make([]agyFactoryAsset, 0, 2)
	for _, relative := range []string{
		"factory.json",
		filepath.Join("workstations", "process", "AGENTS.md"),
	} {
		sourcePath := filepath.Join(sourceDir, relative)
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, err
		}
		assets = append(assets, agyFactoryAsset{
			relative: relative,
			data:     data,
			mode:     info.Mode().Perm(),
		})
	}
	return assets, nil
}

func copyAgyFactoryDirectory(assets []agyFactoryAsset, targetDir string) error {
	// The worker file is authored immediately afterward for the case-specific
	// model, and the checked-in fixture contains no other runtime assets. Copy
	// only the immutable topology and workstation prompt to avoid walking and
	// rewriting unused fixture entries for each shared-process scenario.
	for _, asset := range assets {
		targetPath := filepath.Join(targetDir, asset.relative)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, asset.data, asset.mode); err != nil {
			return err
		}
	}
	return nil
}

func readAgyResponseEvents(
	t testing.TB,
	run *agySharedScenarioRun,
	timeout time.Duration,
	selector string,
) []factoryapi.FactoryResponseEvent {
	t.Helper()
	if run == nil || run.stream == nil {
		t.Fatalf("AGY %q response stream is missing", selector)
	}
	want := 2
	if selector == agySharedTimeoutSelector {
		want = 18
	}
	events := make([]factoryapi.FactoryResponseEvent, 0)
	for len(events) < want {
		result := run.stream.TryNextFrameResult(timeout)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			run.recordStreamReadResult(result)
			t.Fatalf("AGY %q response stream ended before frame %d/%d: %s", selector, len(events)+1, want, result.Diagnostic())
		}
		events = append(events, result.Frame.Event)
	}
	return events
}

func assertAgyResponseEventStreamClosed(
	t testing.TB,
	run *agySharedScenarioRun,
	timeout time.Duration,
	selector string,
	want int,
) {
	t.Helper()
	if err := run.closeSession(context.Background()); err != nil {
		t.Fatalf("AGY %q session cleanup before response stream close: %v", selector, err)
	}
	terminal := run.stream.TryNextFrameResult(timeout)
	run.recordStreamReadResult(terminal)
	switch terminal.Outcome {
	case support.FactoryResponseEventStreamOutcomeEOF:
		// The session-owned response store completes after the terminal Work
		// projection; EOF is the no-extra-frame postcondition for this golden.
	case support.FactoryResponseEventStreamOutcomeFrame:
		t.Fatalf("AGY %q response stream emitted extra frame after %d expected frames: %#v", selector, want, terminal.Frame.Event)
	default:
		t.Fatalf("AGY %q response stream did not close normally after %d frames: %s", selector, want, terminal.Diagnostic())
	}
}

func assertAgySessionObservations(
	t *testing.T,
	scenario *agySharedScenario,
	sessionID string,
	submitted factoryapi.SubmitWorkResponse,
	factoryEvents []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
) {
	t.Helper()
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("AGY %q submitted Work has no ID", scenario.selector)
	}
	support.AssertSingleWorkRequestEvent(t, factoryEvents, submitted.RequestId, workID, "task")
	assertAgyFactoryEventSequence(t, scenario.selector, factoryEvents)
	assertAgyResponseEventTopology(t, scenario.selector, responseEvents)
	for _, event := range factoryEvents {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("AGY %q Factory Event scope = %#v, want session %q", scenario.selector, event.Context, sessionID)
		}
	}
	for _, event := range responseEvents {
		if event.FactorySessionId != sessionID {
			t.Fatalf("AGY %q response event session = %q, want %q", scenario.selector, event.FactorySessionId, sessionID)
		}
		if strings.TrimSpace(event.RunId) == "" {
			t.Fatalf("AGY %q response event = %#v, want run identity", scenario.selector, event)
		}
	}
}

func assertAgyFactoryEventSequence(t testing.TB, selector string, events []factoryapi.FactoryEvent) {
	t.Helper()
	want := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeRunRequest,
		factoryapi.FactoryEventTypeInitialStructureRequest,
		factoryapi.FactoryEventTypeSessionStarted,
		factoryapi.FactoryEventTypeFactoryStateResponse,
		factoryapi.FactoryEventTypeWorkRequest,
	}
	attempt := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeAgentRunResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
	}
	if selector == agySharedTimeoutSelector {
		for i := 0; i < 3; i++ {
			want = append(want, attempt...)
		}
	} else {
		want = append(want, attempt...)
	}
	if len(events) != len(want) {
		t.Fatalf("AGY %q Factory Event count = %d, want %d; types=%v", selector, len(events), len(want), agyFactoryEventTypes(events))
	}
	for index, event := range events {
		if event.Type != want[index] {
			t.Fatalf("AGY %q Factory Event[%d] = %q, want %q; types=%v", selector, index, event.Type, want[index], agyFactoryEventTypes(events))
		}
	}
}

func agyFactoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

type agySharedCommandRoute struct {
	selector     string
	workDir      string
	runner       platformprocess.CommandRunner
	runStreaming func(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	calls        *atomic.Int64
}

// agySharedCommandRouter is immutable after freeze. WorkDir is the only
// selector used during execution, so no test-order or mutable scenario state
// can redirect a provider call to the other golden.
type agySharedCommandRouter struct {
	routeMu         sync.Mutex
	routes          map[string]agySharedCommandRoute
	publishedRoutes atomic.Value
	frozen          bool
	released        atomic.Bool

	lifecycleMu sync.Mutex
	active      atomic.Int64
}

func newAgySharedCommandRouter() *agySharedCommandRouter {
	router := &agySharedCommandRouter{routes: make(map[string]agySharedCommandRoute)}
	router.publishedRoutes.Store(map[string]agySharedCommandRoute{})
	return router
}

func (router *agySharedCommandRouter) register(selector, workDir string, runner platformprocess.CommandRunner) error {
	selector = strings.TrimSpace(selector)
	workDir = strings.TrimSpace(workDir)
	if selector == "" || workDir == "" || runner == nil {
		return fmt.Errorf("AGY route selector, WorkDir, and runner are required")
	}
	absolute, err := filepath.Abs(filepath.Clean(workDir))
	if err != nil {
		return fmt.Errorf("normalize AGY route WorkDir: %w", err)
	}
	router.routeMu.Lock()
	defer router.routeMu.Unlock()
	if router.frozen {
		return fmt.Errorf("AGY routes are already frozen")
	}
	if router.released.Load() {
		return fmt.Errorf("AGY routes have been released")
	}
	if _, exists := router.routes[absolute]; exists {
		return fmt.Errorf("AGY WorkDir route %q is already registered", absolute)
	}
	for _, route := range router.routes {
		if route.selector == selector {
			return fmt.Errorf("AGY route selector %q is already registered", selector)
		}
	}
	route := agySharedCommandRoute{
		selector: selector, workDir: absolute, runner: runner, calls: &atomic.Int64{},
	}
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		route.runStreaming = streaming.RunStreaming
	}
	router.routes[absolute] = route
	router.publishRoutesLocked()
	return nil
}

func (router *agySharedCommandRouter) freeze() error {
	router.routeMu.Lock()
	defer router.routeMu.Unlock()
	if router.released.Load() {
		return fmt.Errorf("AGY routes have been released")
	}
	if len(router.routes) == 0 {
		return fmt.Errorf("AGY route table is empty")
	}
	router.publishRoutesLocked()
	router.frozen = true
	return nil
}

func (router *agySharedCommandRouter) releaseAll() error {
	router.routeMu.Lock()
	defer router.routeMu.Unlock()
	router.lifecycleMu.Lock()
	defer router.lifecycleMu.Unlock()
	if active := router.active.Load(); active != 0 {
		return fmt.Errorf("cannot release AGY routes with %d active calls", active)
	}
	router.released.Store(true)
	router.routes = make(map[string]agySharedCommandRoute)
	router.publishRoutesLocked()
	return nil
}

func (router *agySharedCommandRouter) routeCount() int {
	return len(router.publishedRoutes.Load().(map[string]agySharedCommandRoute))
}

func (router *agySharedCommandRouter) activeCallCount() int {
	return int(router.active.Load())
}

func (router *agySharedCommandRouter) routeCallCount(selector string) int {
	selector = strings.TrimSpace(selector)
	routes := router.publishedRoutes.Load().(map[string]agySharedCommandRoute)
	for _, route := range routes {
		if route.selector == selector && route.calls != nil {
			return int(route.calls.Load())
		}
	}
	return 0
}

func (router *agySharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	route, err := router.beginCall(ctx, request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	defer router.endCall()
	return route.runner.Run(ctx, request)
}

// RunStreaming keeps the immutable AGY doubles on the streaming path. All
// production routing and lifecycle checks remain shared with Run; only the
// optional output-forwarding capability differs.
func (router *agySharedCommandRouter) RunStreaming(
	ctx context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	route, err := router.beginCall(ctx, request)
	if err != nil {
		return platformprocess.CommandResult{}, err
	}
	defer router.endCall()
	if route.runStreaming != nil {
		return route.runStreaming(ctx, request, observer)
	}
	result, err := route.runner.Run(ctx, request)
	if observer != nil {
		if len(result.Stdout) > 0 {
			observer(platformprocess.OutputStreamStdout, append([]byte(nil), result.Stdout...))
		}
		if len(result.Stderr) > 0 {
			observer(platformprocess.OutputStreamStderr, append([]byte(nil), result.Stderr...))
		}
	}
	return result, err
}

func (router *agySharedCommandRouter) beginCall(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (agySharedCommandRoute, error) {
	rawWorkDir := strings.TrimSpace(request.WorkDir)
	if rawWorkDir == "" {
		return agySharedCommandRoute{}, fmt.Errorf("AGY command WorkDir is required")
	}
	routes := router.publishedRoutes.Load().(map[string]agySharedCommandRoute)
	workDir := rawWorkDir
	route, ok := routes[workDir]
	if !ok {
		var err error
		workDir, err = filepath.Abs(filepath.Clean(rawWorkDir))
		if err != nil {
			return agySharedCommandRoute{}, fmt.Errorf("normalize AGY command WorkDir: %w", err)
		}
		route, ok = routes[workDir]
	}
	if !ok {
		return agySharedCommandRoute{}, fmt.Errorf("no AGY route matched WorkDir %q", workDir)
	}
	if request.Command != agySharedCommand {
		return agySharedCommandRoute{}, fmt.Errorf("AGY route %q received command %q", route.selector, request.Command)
	}
	if err := ctx.Err(); err != nil {
		return agySharedCommandRoute{}, err
	}
	router.lifecycleMu.Lock()
	if router.released.Load() {
		router.lifecycleMu.Unlock()
		return agySharedCommandRoute{}, fmt.Errorf("AGY route table is released")
	}
	router.active.Add(1)
	router.lifecycleMu.Unlock()
	if route.calls != nil {
		route.calls.Add(1)
	}
	return route, nil
}

func (router *agySharedCommandRouter) publishRoutesLocked() {
	snapshot := make(map[string]agySharedCommandRoute, len(router.routes))
	for workDir, route := range router.routes {
		snapshot[workDir] = route
	}
	router.publishedRoutes.Store(snapshot)
}

func (router *agySharedCommandRouter) endCall() {
	// Activity is read independently from the request ledger, so completion of
	// each retry need not reacquire the ledger mutex.
	router.active.Add(-1)
}

func assertAgyRoutesRejectInvalidRegistrations(router *agySharedCommandRouter, rootDir, registeredWorkDir string) error {
	if err := router.register("", filepath.Join(rootDir, "invalid"), testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("empty AGY selector was accepted")
	}
	if err := router.register("duplicate-workdir", registeredWorkDir, testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("duplicate AGY WorkDir was accepted")
	}
	if err := router.register(agySharedSuccessSelector, filepath.Join(rootDir, "duplicate-selector"), testutil.NewProviderCommandRunner()); err == nil {
		return fmt.Errorf("duplicate AGY selector was accepted")
	}
	if got := router.routeCount(); got != 2 {
		return fmt.Errorf("AGY route count after invalid registration = %d, want two", got)
	}
	unknown := filepath.Join(rootDir, "unknown-route")
	_, err := router.Run(context.Background(), platformprocess.CommandRequest{
		Command: agySharedCommand,
		WorkDir: unknown,
		Stdin:   []byte("sensitive AGY input"),
		Env:     []string{"AGY_SECRET=sensitive"},
	})
	if err == nil {
		return fmt.Errorf("unknown AGY WorkDir was accepted")
	}
	if strings.Contains(err.Error(), "sensitive") {
		return fmt.Errorf("unknown AGY route diagnostic leaked sensitive input")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = router.Run(canceled, platformprocess.CommandRequest{
		Command: agySharedCommand, WorkDir: rootDir,
	})
	if err == nil {
		return fmt.Errorf("canceled AGY command was accepted")
	}
	return nil
}
