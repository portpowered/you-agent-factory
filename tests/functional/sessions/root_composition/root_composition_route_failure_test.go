package root_composition_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func TestRootCompositionRoutesFailClosed(t *testing.T) {
	t.Parallel()
	t.Run("missing route rejects the command without invoking the runner", testRootCompositionMissingRoute)
	t.Run("duplicate and overlapping registration preserve the first route", testRootCompositionDuplicateRoute)
	t.Run("disjoint selectors route concurrently and reject unowned paths", testRootCompositionDisjointRoutes)
}

func testRootCompositionMissingRoute(t *testing.T) {
	registry := newRootCompositionRouteRegistry()
	runner := &rootCompositionCountingCommandRunner{}
	commandRouter := registry.commandRunner("provider")
	registryRoute, err := registry.register(rootCompositionRouteSpec{
		label:          "owned",
		homeDir:        filepath.Join(t.TempDir(), "home"),
		workingDir:     filepath.Join(t.TempDir(), "factory"),
		providerRunner: runner,
	})
	if err != nil {
		t.Fatalf("register route: %v", err)
	}
	if err := registry.unregister(registryRoute); err != nil {
		t.Fatalf("unregister route: %v", err)
	}

	_, err = commandRouter.Run(context.Background(), platformprocess.CommandRequest{WorkDir: filepath.Join(t.TempDir(), "not-owned")})
	if !errors.Is(err, errRootCompositionRouteNotFound) {
		t.Fatalf("missing route error = %v, want route-not-found", err)
	}
	if got := runner.calls.Load(); got != 0 {
		t.Fatalf("missing route invoked runner %d times, want 0", got)
	}
}

func testRootCompositionDuplicateRoute(t *testing.T) {
	registry := newRootCompositionRouteRegistry()
	root := t.TempDir()
	first, err := registry.register(rootCompositionRouteSpec{
		label:      "first",
		homeDir:    filepath.Join(root, "home"),
		workingDir: filepath.Join(root, "factory"),
	})
	if err != nil {
		t.Fatalf("register first route: %v", err)
	}
	if _, err := registry.register(rootCompositionRouteSpec{
		label:      "first",
		homeDir:    filepath.Join(root, "other-home"),
		workingDir: filepath.Join(root, "other-factory"),
	}); !errors.Is(err, errRootCompositionRouteDuplicate) {
		t.Fatalf("duplicate route error = %v, want duplicate", err)
	}
	if _, err := registry.register(rootCompositionRouteSpec{
		label:      "nested",
		homeDir:    filepath.Join(root, "nested-home"),
		workingDir: filepath.Join(root, "factory", "nested"),
	}); !errors.Is(err, errRootCompositionRouteOverlap) {
		t.Fatalf("overlapping route error = %v, want overlap", err)
	}
	if got := registry.count(); got != 1 {
		t.Fatalf("registered route count = %d, want 1", got)
	}
	selected, err := registry.routeForPath(filepath.Join(first.workingDir, "input.json"))
	if err != nil || selected != first {
		t.Fatalf("selected route = %v, error = %v; want first route", selected, err)
	}
	if err := registry.unregister(first); err != nil {
		t.Fatalf("unregister first route: %v", err)
	}
}

func testRootCompositionDisjointRoutes(t *testing.T) {
	registry := newRootCompositionRouteRegistry()
	root := t.TempDir()
	first, err := registry.register(rootCompositionRouteSpec{
		label:      "first",
		homeDir:    filepath.Join(root, "home-first"),
		workingDir: filepath.Join(root, "factory-first"),
	})
	if err != nil {
		t.Fatalf("register first route: %v", err)
	}
	second, err := registry.register(rootCompositionRouteSpec{
		label:      "second",
		homeDir:    filepath.Join(root, "home-second"),
		workingDir: filepath.Join(root, "factory-second"),
	})
	if err != nil {
		t.Fatalf("register second route: %v", err)
	}
	if selected, err := registry.routeForEffectPath(first.workingDir); err != nil || selected != first {
		t.Fatalf("first selector = %v, error = %v; want first route", selected, err)
	}
	if selected, err := registry.routeForEffectPath(second.workingDir); err != nil || selected != second {
		t.Fatalf("second selector = %v, error = %v; want second route", selected, err)
	}
	if _, err := registry.routeForEffectPath(filepath.Join(root, "not-owned")); !errors.Is(err, errRootCompositionRouteNotFound) {
		t.Fatalf("unowned effect error = %v, want route-not-found", err)
	}
	if err := registry.unregister(first); err != nil {
		t.Fatalf("unregister first route: %v", err)
	}
	if err := registry.unregister(second); err != nil {
		t.Fatalf("unregister second route: %v", err)
	}
	closed, err := registry.register(rootCompositionRouteSpec{
		label:      "closed",
		homeDir:    filepath.Join(root, "home-closed"),
		workingDir: filepath.Join(root, "factory-closed"),
	})
	if err != nil {
		t.Fatalf("register closed-route witness: %v", err)
	}
	if err := registry.unregister(closed); err != nil {
		t.Fatalf("unregister closed-route witness: %v", err)
	}
	if _, err := registry.routeForPath(closed.workingDir); !errors.Is(err, errRootCompositionRouteNotFound) {
		t.Fatalf("closed route selection error = %v, want route-not-found", err)
	}
}
