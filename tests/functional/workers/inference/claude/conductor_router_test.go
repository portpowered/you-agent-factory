package claude

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

type claudeCommandRoute struct {
	selector string
	label    string
	runner   *claudeScenarioCommandRunner
}

// claudeCommandRouter is immutable after construction. Its map is populated
// before root.BuildProcess and only read by concurrent provider attempts.
type claudeCommandRouter struct {
	routes map[string]claudeCommandRoute

	mu    sync.Mutex
	calls []claudeRoutedCommand
}

type claudeRoutedCommand struct {
	selector string
	request  platformprocess.CommandRequest
}

func newClaudeCommandRouter(routes []claudeCommandRoute) (*claudeCommandRouter, error) {
	indexed := make(map[string]claudeCommandRoute, len(routes))
	for _, route := range routes {
		selector := filepath.Clean(strings.TrimSpace(route.selector))
		if selector == "." || selector == "" {
			return nil, fmt.Errorf("Claude scenario selector is required")
		}
		if route.runner == nil {
			return nil, fmt.Errorf("Claude scenario selector %q has no command runner", selector)
		}
		if _, exists := indexed[selector]; exists {
			return nil, fmt.Errorf("duplicate Claude scenario selector %q", selector)
		}
		route.selector = selector
		indexed[selector] = route
	}
	return &claudeCommandRouter{routes: indexed}, nil
}

func (router *claudeCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector := filepath.Clean(strings.TrimSpace(request.WorkDir))
	route, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown Claude scenario selector %q; refusing to consume another route", request.WorkDir)
	}
	router.mu.Lock()
	router.calls = append(router.calls, claudeRoutedCommand{
		selector: route.selector,
		request:  cloneClaudeCommandRequest(request),
	})
	router.mu.Unlock()
	return route.runner.Run(ctx, request)
}

func (router *claudeCommandRouter) callsFor(selector string) []claudeRoutedCommand {
	router.mu.Lock()
	defer router.mu.Unlock()
	selector = filepath.Clean(strings.TrimSpace(selector))
	var calls []claudeRoutedCommand
	for _, call := range router.calls {
		if call.selector == selector {
			call.request = cloneClaudeCommandRequest(call.request)
			calls = append(calls, call)
		}
	}
	return calls
}

func (router *claudeCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

type claudeScenarioCommandRunner struct {
	results []platformprocess.CommandResult
	err     error
	release chan struct{}
	once    sync.Once

	mu       sync.Mutex
	requests []platformprocess.CommandRequest
	active   atomic.Int32
}

func newClaudeScenarioCommandRunner(
	results []platformprocess.CommandResult,
	runErr error,
) *claudeScenarioCommandRunner {
	clonedResults := make([]platformprocess.CommandResult, len(results))
	for index, result := range results {
		clonedResults[index] = cloneClaudeCommandResult(result)
	}
	return &claudeScenarioCommandRunner{
		results: clonedResults,
		err:     runErr,
		release: make(chan struct{}),
	}
}

func (runner *claudeScenarioCommandRunner) Run(
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
	runner.requests = append(runner.requests, cloneClaudeCommandRequest(request))
	resultIndex := len(runner.requests) - 1
	if resultIndex >= len(runner.results) {
		runner.mu.Unlock()
		return platformprocess.CommandResult{}, fmt.Errorf(
			"Claude scenario command result queue exhausted at call %d",
			resultIndex+1,
		)
	}
	result := cloneClaudeCommandResult(runner.results[resultIndex])
	err := runner.err
	runner.mu.Unlock()
	return result, err
}

func (runner *claudeScenarioCommandRunner) Release() {
	if runner == nil || runner.release == nil {
		return
	}
	runner.once.Do(func() { close(runner.release) })
}

func (runner *claudeScenarioCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = cloneClaudeCommandRequest(request)
	}
	return requests
}

func (runner *claudeScenarioCommandRunner) CallCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return len(runner.requests)
}

func (runner *claudeScenarioCommandRunner) ActiveCallCount() int {
	return int(runner.active.Load())
}

func cloneClaudeCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func cloneClaudeCommandResult(result platformprocess.CommandResult) platformprocess.CommandResult {
	result.Stdout = append([]byte(nil), result.Stdout...)
	result.Stderr = append([]byte(nil), result.Stderr...)
	return result
}

type claudeIdentityGenerator struct {
	sessions      atomic.Uint64
	runtimes      atomic.Uint64
	responseEvent atomic.Uint64
}

func (generator *claudeIdentityGenerator) nextSessionID() string {
	// Explicit live sessions persist this value directly, so use the UUID form
	// accepted by the durable-session store while keeping allocation deterministic.
	return fmt.Sprintf("00000000-0000-4000-8000-%012x", generator.sessions.Add(1))
}

func (generator *claudeIdentityGenerator) nextRuntimeID() string {
	return fmt.Sprintf("c03-claude-runtime-%d", generator.runtimes.Add(1))
}

func (generator *claudeIdentityGenerator) nextResponseEventID() string {
	return fmt.Sprintf("c03-claude-response-event-%d", generator.responseEvent.Add(1))
}

func (generator *claudeIdentityGenerator) sessionCount() uint64 {
	return generator.sessions.Load()
}

var _ platformprocess.CommandRunner = (*claudeCommandRouter)(nil)
