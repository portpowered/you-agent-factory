package agy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

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

func assertAgyFactorySessionDeleted(baseURL, sessionID string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	// Keep the default transport pool reusable for the next public observation;
	// the API server explicitly closes client connections during final shutdown.
	client := &http.Client{Timeout: agySharedScenarioTimeout}
	response, err := client.Get(endpoint)
	if err != nil {
		return fmt.Errorf("GET deleted AGY Factory Session %q: %w", sessionID, err)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil {
		return errors.Join(readErr, closeErr)
	}
	if response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("GET deleted AGY Factory Session %q status = %d, want 404: %s", sessionID, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return closeErr
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

func copyAgyFactoryDirectory(sourceDir, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode().Perm())
	})
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
	selector string
	workDir  string
	runner   platformprocess.CommandRunner
}

// agySharedCommandRouter is immutable after freeze. WorkDir is the only
// selector used during execution, so no test-order or mutable scenario state
// can redirect a provider call to the other golden.
type agySharedCommandRouter struct {
	mu       sync.Mutex
	routes   map[string]agySharedCommandRoute
	requests []platformprocess.CommandRequest
	active   int
	frozen   bool
	released bool
}

func newAgySharedCommandRouter() *agySharedCommandRouter {
	return &agySharedCommandRouter{routes: make(map[string]agySharedCommandRoute)}
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
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.frozen {
		return fmt.Errorf("AGY routes are already frozen")
	}
	if router.released {
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
	router.routes[absolute] = agySharedCommandRoute{selector: selector, workDir: absolute, runner: runner}
	return nil
}

func (router *agySharedCommandRouter) freeze() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.released {
		return fmt.Errorf("AGY routes have been released")
	}
	if len(router.routes) == 0 {
		return fmt.Errorf("AGY route table is empty")
	}
	router.frozen = true
	return nil
}

func (router *agySharedCommandRouter) releaseAll() error {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.active != 0 {
		return fmt.Errorf("cannot release AGY routes with %d active calls", router.active)
	}
	router.routes = make(map[string]agySharedCommandRoute)
	router.released = true
	return nil
}

func (router *agySharedCommandRouter) routeCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.routes)
}

func (router *agySharedCommandRouter) activeCallCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return router.active
}

func (router *agySharedCommandRouter) requestCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.requests)
}

func (router *agySharedCommandRouter) requestsSince(start int) []platformprocess.CommandRequest {
	router.mu.Lock()
	defer router.mu.Unlock()
	if start < 0 {
		start = 0
	}
	if start > len(router.requests) {
		start = len(router.requests)
	}
	requests := make([]platformprocess.CommandRequest, len(router.requests)-start)
	for index, request := range router.requests[start:] {
		requests[index] = cloneAgyCommandRequest(request)
	}
	return requests
}

func (router *agySharedCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	rawWorkDir := strings.TrimSpace(request.WorkDir)
	if rawWorkDir == "" {
		return platformprocess.CommandResult{}, fmt.Errorf("AGY command WorkDir is required")
	}
	workDir, err := filepath.Abs(filepath.Clean(rawWorkDir))
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("normalize AGY command WorkDir: %w", err)
	}
	router.mu.Lock()
	if router.released {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route table is released")
	}
	route, ok := router.routes[workDir]
	if !ok {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("no AGY route matched WorkDir %q", workDir)
	}
	if request.Command != agySharedCommand {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf("AGY route %q received command %q", route.selector, request.Command)
	}
	if err := ctx.Err(); err != nil {
		router.mu.Unlock()
		return platformprocess.CommandResult{}, err
	}
	router.requests = append(router.requests, cloneAgyCommandRequest(request))
	router.active++
	router.mu.Unlock()
	defer func() {
		router.mu.Lock()
		router.active--
		router.mu.Unlock()
	}()
	return route.runner.Run(ctx, request)
}

func cloneAgyCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
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
