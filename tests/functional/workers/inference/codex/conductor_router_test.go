package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type codexCommandRoute struct {
	selector string
	label    string
	runner   *codexScenarioCommandRunner
}

// codexCommandRouter is immutable after construction. Its map is populated
// before root.BuildProcess and only read by concurrent provider attempts.
type codexCommandRouter struct {
	routes map[string]codexCommandRoute

	mu    sync.Mutex
	calls []codexRoutedCommand
}

type codexRoutedCommand struct {
	selector string
	request  platformprocess.CommandRequest
}

func newCodexCommandRouter(routes []codexCommandRoute) (*codexCommandRouter, error) {
	indexed := make(map[string]codexCommandRoute, len(routes))
	for _, route := range routes {
		selector := filepath.Clean(strings.TrimSpace(route.selector))
		if selector == "." || selector == "" {
			return nil, fmt.Errorf("Codex scenario selector is required")
		}
		if route.runner == nil {
			return nil, fmt.Errorf("Codex scenario selector %q has no command runner", selector)
		}
		if _, exists := indexed[selector]; exists {
			return nil, fmt.Errorf("duplicate Codex scenario selector %q", selector)
		}
		route.selector = selector
		indexed[selector] = route
	}
	return &codexCommandRouter{routes: indexed}, nil
}

func (router *codexCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	route, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf(
			"unknown Codex scenario selector %q; refusing to consume another route",
			request.WorkDir,
		)
	}
	router.mu.Lock()
	router.calls = append(router.calls, codexRoutedCommand{
		selector: route.selector,
		request:  cloneCodexCommandRequest(request),
	})
	router.mu.Unlock()
	return route.runner.Run(ctx, request)
}

func (router *codexCommandRouter) callsFor(selector string) []codexRoutedCommand {
	router.mu.Lock()
	defer router.mu.Unlock()
	selector = filepath.Clean(strings.TrimSpace(selector))
	var calls []codexRoutedCommand
	for _, call := range router.calls {
		if call.selector != selector {
			continue
		}
		call.request = cloneCodexCommandRequest(call.request)
		calls = append(calls, call)
	}
	return calls
}

func (router *codexCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

// codexScenarioCommandRunner holds each scenario's controlled result queue
// behind a release gate. This makes parallel sessions order-independent while
// retaining the exact ProviderCommandRunner edge used by the application.
type codexScenarioCommandRunner struct {
	results []platformprocess.CommandResult
	err     error
	release chan struct{}
	once    sync.Once

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	active   atomic.Int32
}

func newCodexScenarioCommandRunner(
	results []platformprocess.CommandResult,
	runErr error,
) *codexScenarioCommandRunner {
	clonedResults := make([]platformprocess.CommandResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneCodexCommandResult(result)
	}
	return &codexScenarioCommandRunner{
		results: clonedResults,
		err:     runErr,
		release: make(chan struct{}),
	}
}

func (runner *codexScenarioCommandRunner) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	runner.active.Add(1)
	defer runner.active.Add(-1)
	if runner.release != nil {
		select {
		case <-runner.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	runner.mu.Lock()
	runner.requests = append(runner.requests, cloneCodexCommandRequest(request))
	resultIndex := len(runner.requests) - 1
	if resultIndex >= len(runner.results) {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Codex scenario command result queue exhausted at call %d",
			resultIndex+1,
		)
	}
	result := cloneCodexCommandResult(runner.results[resultIndex])
	err := runner.err
	runner.mu.Unlock()
	return result, err
}

func (runner *codexScenarioCommandRunner) Release() {
	if runner == nil || runner.release == nil {
		return
	}
	runner.once.Do(func() { close(runner.release) })
}

func (runner *codexScenarioCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneCodexCommandRequest(request)
	}
	return requests
}

func (runner *codexScenarioCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *codexScenarioCommandRunner) ActiveCallCount() int {
	return int(runner.active.Load())
}

func cloneCodexCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneCodexCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type codexIdentityGenerator struct {
	sessions      atomic.Uint64
	runtimes      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *codexIdentityGenerator) nextSessionID() string {
	// Explicit live sessions persist this value directly, so use the UUID form
	// accepted by the durable-session store while keeping allocation deterministic.
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *codexIdentityGenerator) nextRuntimeID() string {
	return fmt.Sprintf("c04-codex-runtime-%d", generator.runtimes.Add(1))
}

func (generator *codexIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("c04-codex-response-event-%d", generator.responseEvent.Add(1))
}

func (generator *codexIdentityGenerator) sessionCount() uint64 {
	return generator.sessions.Load()
}

var _ platformprocess.CommandRunner = (*codexCommandRouter)(nil)
