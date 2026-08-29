package script_test

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type scriptCommandRoute struct {
	selector string
	runner   platformprocess.CommandRunner
}

type scriptCommandRouter struct {
	routes map[string]scriptCommandRoute

	mu    sync.Mutex
	calls []scriptRoutedCommand
}

type scriptRoutedCommand struct {
	selector string
	request  platformprocess.CommandRequest
}

// newScriptCommandRouter freezes every route before the root process is built.
// The map is read-only during execution; only the diagnostic call ledger is
// synchronized for concurrent scenario observations.
func newScriptCommandRouter(routes []scriptCommandRoute) (*scriptCommandRouter, error) {
	indexed := make(map[string]scriptCommandRoute, len(routes))
	for _, route := range routes {
		selector, err := normalizeScriptRouteSelector(route.selector)
		if err != nil {
			return nil, err
		}
		if route.runner == nil {
			return nil, fmt.Errorf("script route %q has no command runner", selector)
		}
		if _, exists := indexed[selector]; exists {
			return nil, fmt.Errorf("duplicate script route selector %q", scriptSelectorContext(route.selector))
		}
		route.selector = selector
		indexed[selector] = route
	}
	return &scriptCommandRouter{routes: indexed}, nil
}

func (router *scriptCommandRouter) Run(
	ctx context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	selector, err := normalizeScriptRouteSelector(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("script route selector %q is invalid", scriptSelectorContext(request.WorkDir))
	}
	route, ok := router.routes[selector]
	if !ok {
		return platformprocess.CommandResult{}, fmt.Errorf("unknown script route selector %q", scriptSelectorContext(request.WorkDir))
	}
	if err := ctx.Err(); err != nil {
		return platformprocess.CommandResult{}, err
	}
	router.mu.Lock()
	router.calls = append(router.calls, scriptRoutedCommand{
		selector: selector,
		request:  cloneScriptCommandRequest(request),
	})
	router.mu.Unlock()
	return route.runner.Run(ctx, request)
}

func (router *scriptCommandRouter) callsFor(selector string) []scriptRoutedCommand {
	cleaned, err := normalizeScriptRouteSelector(selector)
	if err != nil {
		return nil
	}
	router.mu.Lock()
	defer router.mu.Unlock()
	calls := make([]scriptRoutedCommand, 0)
	for _, call := range router.calls {
		if call.selector != cleaned {
			continue
		}
		call.request = cloneScriptCommandRequest(call.request)
		calls = append(calls, call)
	}
	return calls
}

func (router *scriptCommandRouter) callCount() int {
	router.mu.Lock()
	defer router.mu.Unlock()
	return len(router.calls)
}

func (router *scriptCommandRouter) routeCount() int {
	return len(router.routes)
}

func normalizeScriptRouteSelector(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", fmt.Errorf("script route selector is required")
	}
	abs, err := filepath.Abs(trimmed)
	if err != nil {
		return "", fmt.Errorf("normalize script route selector: %w", err)
	}
	cleaned := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		cleaned = strings.ToLower(cleaned)
	}
	return cleaned, nil
}

func cleanScriptRouteSelector(path string) string {
	cleaned, _ := normalizeScriptRouteSelector(path)
	return cleaned
}

func scriptSelectorContext(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "<empty>"
	}
	base := filepath.Base(filepath.Clean(trimmed))
	if base == "." || base == string(filepath.Separator) || base == "\\" {
		return "<root>"
	}
	return base
}

func cloneScriptCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

func TestScriptCommandRouterRejectsUnknownAndDuplicateSelectors(t *testing.T) {
	firstSelector := t.TempDir()
	runner := support.NewRecordingCommandRunner("must-not-run")
	router, err := newScriptCommandRouter([]scriptCommandRoute{{
		selector: firstSelector,
		runner:   runner,
	}})
	if err != nil {
		t.Fatalf("newScriptCommandRouter: %v", err)
	}

	if _, err := newScriptCommandRouter([]scriptCommandRoute{
		{selector: firstSelector, runner: runner},
		{selector: firstSelector, runner: runner},
	}); err == nil {
		t.Fatal("duplicate script selector was accepted")
	}

	secret := "script-router-secret"
	unknown := filepath.Join(t.TempDir(), "unknown-selector")
	_, err = router.Run(context.Background(), platformprocess.CommandRequest{
		Command: "echo",
		Args:    []string{secret},
		Env:     []string{"ROUTER_SECRET=" + secret},
		WorkDir: unknown,
	})
	if err == nil {
		t.Fatal("unknown script selector was accepted")
	}
	if !strings.Contains(err.Error(), "unknown-selector") {
		t.Fatalf("unknown selector error = %v, want sanitized selector context", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), filepath.Dir(unknown)) {
		t.Fatalf("unknown selector error leaked request or path context: %v", err)
	}
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("runner calls after unknown selector = %d, want zero", got)
	}
	if got := router.callCount(); got != 0 {
		t.Fatalf("router calls after unknown selector = %d, want zero", got)
	}
}

var _ platformprocess.CommandRunner = (*scriptCommandRouter)(nil)
