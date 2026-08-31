package root_composition_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type rootCompositionRouteSpec struct {
	label           string
	homeDir         string
	workingDir      string
	extraPaths      []string
	providerRoot    string
	providerWorker  string
	providerStation string
	api             *support.ProcessAPIServer
	apiStarter      func(context.Context, platformhttpserver.StartRequest) error
	provider        providers.Service
	providerRunner  platformprocess.CommandRunner
	scriptRunner    platformprocess.CommandRunner
}

type rootCompositionRoute struct {
	label           string
	homeDir         string
	workingDir      string
	selectors       []string
	providerRoot    string
	providerWorker  string
	providerStation string
	api             *support.ProcessAPIServer
	apiStarter      func(context.Context, platformhttpserver.StartRequest) error
	provider        providers.Service
	providerRunner  platformprocess.CommandRunner
	scriptRunner    platformprocess.CommandRunner
	cleanup         *rootCompositionCleanupLedger

	mu             sync.Mutex
	temporaryFiles map[string]struct{}
	closed         bool
}

type rootCompositionRouteRegistry struct {
	mu            sync.RWMutex
	routes        map[string]*rootCompositionRoute
	closed        bool
	unmatched     atomic.Int64
	providerCalls atomic.Int64
	scriptCalls   atomic.Int64
}

func newRootCompositionRouteRegistry() *rootCompositionRouteRegistry {
	return &rootCompositionRouteRegistry{
		routes: make(map[string]*rootCompositionRoute),
	}
}

func (registry *rootCompositionRouteRegistry) register(spec rootCompositionRouteSpec) (*rootCompositionRoute, error) {
	label := strings.TrimSpace(spec.label)
	if label == "" {
		return nil, fmt.Errorf("%w: label is empty", errRootCompositionRouteDuplicate)
	}
	homeDir, err := normalizeRootCompositionPath(spec.homeDir)
	if err != nil {
		return nil, fmt.Errorf("route %q home directory: %w", label, err)
	}
	workingDir, err := normalizeRootCompositionPath(spec.workingDir)
	if err != nil {
		return nil, fmt.Errorf("route %q working directory: %w", label, err)
	}
	selectors := []string{homeDir, workingDir}
	providerRoot := ""
	if strings.TrimSpace(spec.providerRoot) != "" {
		providerRoot, err = normalizeRootCompositionPath(spec.providerRoot)
		if err != nil {
			return nil, fmt.Errorf("route %q provider root: %w", label, err)
		}
	}
	for _, extraPath := range spec.extraPaths {
		normalized, err := normalizeRootCompositionPath(extraPath)
		if err != nil {
			return nil, fmt.Errorf("route %q extra path: %w", label, err)
		}
		selectors = append(selectors, normalized)
	}
	selectors = uniqueRootCompositionPaths(selectors)

	route := &rootCompositionRoute{
		label:           label,
		homeDir:         homeDir,
		workingDir:      workingDir,
		selectors:       selectors,
		providerRoot:    providerRoot,
		providerWorker:  strings.TrimSpace(spec.providerWorker),
		providerStation: strings.TrimSpace(spec.providerStation),
		api:             spec.api,
		apiStarter:      spec.apiStarter,
		provider:        spec.provider,
		providerRunner:  spec.providerRunner,
		scriptRunner:    spec.scriptRunner,
		cleanup:         newRootCompositionCleanupLedger(),
		temporaryFiles:  make(map[string]struct{}),
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return nil, errRootCompositionRouteRegistration
	}
	if _, exists := registry.routes[label]; exists {
		return nil, fmt.Errorf("%w: %q", errRootCompositionRouteDuplicate, label)
	}
	for _, existing := range registry.routes {
		if rootCompositionSelectorsOverlap(existing.selectors, route.selectors) {
			return nil, fmt.Errorf("%w: %q overlaps %q", errRootCompositionRouteOverlap, label, existing.label)
		}
	}
	registry.routes[label] = route
	return route, nil
}

func (registry *rootCompositionRouteRegistry) unregister(route *rootCompositionRoute) error {
	if route == nil {
		return nil
	}
	registry.mu.Lock()
	if current, exists := registry.routes[route.label]; !exists || current != route {
		registry.mu.Unlock()
		return nil
	}
	delete(registry.routes, route.label)
	registry.mu.Unlock()

	var errs []error
	if err := route.cleanup.cleanup(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup route %q: %w", route.label, err))
	}

	route.mu.Lock()
	route.closed = true
	temporaryFiles := make([]string, 0, len(route.temporaryFiles))
	for path := range route.temporaryFiles {
		temporaryFiles = append(temporaryFiles, path)
	}
	route.temporaryFiles = make(map[string]struct{})
	route.mu.Unlock()

	for _, path := range temporaryFiles {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("remove temporary file %q: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

func (registry *rootCompositionRouteRegistry) requireRegistered(route *rootCompositionRoute) error {
	if route == nil {
		return errRootCompositionRouteNotFound
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	if registry.closed {
		return errRootCompositionRouteRegistration
	}
	current, exists := registry.routes[route.label]
	if !exists || current != route {
		return errRootCompositionRouteNotFound
	}
	return nil
}

func (registry *rootCompositionRouteRegistry) routeForPath(path string) (*rootCompositionRoute, error) {
	normalized, err := normalizeRootCompositionPath(path)
	if err != nil {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: %v", errRootCompositionRouteNotFound, err)
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var matches []*rootCompositionRoute
	for _, route := range registry.routes {
		if !rootCompositionRouteIsOpen(route) {
			continue
		}
		for _, selector := range route.selectors {
			if rootCompositionPathWithin(selector, normalized) {
				matches = append(matches, route)
				break
			}
		}
	}
	if len(matches) == 0 {
		registry.unmatched.Add(1)
		return nil, fmt.Errorf("%w: %q", errRootCompositionRouteNotFound, normalized)
	}
	if len(matches) > 1 {
		registry.unmatched.Add(1)
		labels := make([]string, len(matches))
		for index, match := range matches {
			labels[index] = match.label
		}
		sort.Strings(labels)
		return nil, fmt.Errorf("%w: %s", errRootCompositionRouteAmbiguous, strings.Join(labels, ", "))
	}
	return matches[0], nil
}

func (registry *rootCompositionRouteRegistry) routeForEffectPath(path string) (*rootCompositionRoute, error) {
	return registry.routeForPath(path)
}

func (registry *rootCompositionRouteRegistry) routeForCommand(path string) (*rootCompositionRoute, error) {
	return registry.routeForPath(path)
}

func (registry *rootCompositionRouteRegistry) routeForProvider(request providers.ExecuteRequest) (*rootCompositionRoute, error) {
	path := strings.TrimSpace(request.FactoryDirectory)
	if path == "" {
		path = strings.TrimSpace(request.WorkingDirectory)
	}
	if path != "" {
		if route, err := registry.routeForPath(path); err == nil && route.provider != nil {
			return route, nil
		}
	}

	registry.mu.RLock()
	defer registry.mu.RUnlock()
	var matches []*rootCompositionRoute
	for _, route := range registry.routes {
		if !rootCompositionRouteIsOpen(route) || route.provider == nil || route.providerRoot == "" {
			continue
		}
		if path == "" || !rootCompositionPathWithin(route.providerRoot, path) {
			continue
		}
		if route.providerWorker != "" && !strings.EqualFold(route.providerWorker, request.WorkerType) {
			continue
		}
		if route.providerStation != "" && !strings.EqualFold(route.providerStation, request.WorkstationName) {
			continue
		}
		matches = append(matches, route)
	}
	if len(matches) == 0 {
		return nil, errRootCompositionRouteNotFound
	}
	if len(matches) > 1 {
		labels := make([]string, len(matches))
		for index, match := range matches {
			labels[index] = match.label
		}
		sort.Strings(labels)
		return nil, fmt.Errorf("%w: %s", errRootCompositionRouteAmbiguous, strings.Join(labels, ", "))
	}
	return matches[0], nil
}

func rootCompositionRouteIsOpen(route *rootCompositionRoute) bool {
	if route == nil {
		return false
	}
	route.mu.Lock()
	defer route.mu.Unlock()
	return !route.closed
}

func (registry *rootCompositionRouteRegistry) commandRunner(kind string) platformprocess.CommandRunner {
	return &rootCompositionCommandRouter{registry: registry, kind: kind}
}

func (registry *rootCompositionRouteRegistry) providerOverride() providers.Service {
	return &rootCompositionProviderRouter{
		registry: registry,
	}
}

func (registry *rootCompositionRouteRegistry) unmatchedCount() int64 {
	return registry.unmatched.Load()
}

func (registry *rootCompositionRouteRegistry) count() int {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return len(registry.routes)
}

func (registry *rootCompositionRouteRegistry) cleanup() error {
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return nil
	}
	registry.closed = true
	routes := make([]*rootCompositionRoute, 0, len(registry.routes))
	for _, route := range registry.routes {
		routes = append(routes, route)
	}
	registry.routes = make(map[string]*rootCompositionRoute)
	registry.mu.Unlock()

	var errs []error
	for _, route := range routes {
		if err := route.cleanup.cleanup(); err != nil {
			errs = append(errs, fmt.Errorf("cleanup route %q: %w", route.label, err))
		}
		route.mu.Lock()
		route.closed = true
		temporaryFiles := make([]string, 0, len(route.temporaryFiles))
		for path := range route.temporaryFiles {
			temporaryFiles = append(temporaryFiles, path)
		}
		route.temporaryFiles = make(map[string]struct{})
		route.mu.Unlock()
		for _, path := range temporaryFiles {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, fmt.Errorf("remove temporary file %q: %w", path, err))
			}
		}
	}
	return errors.Join(errs...)
}

func normalizeRootCompositionPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("path is empty")
	}
	absPath, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", err
	}
	return filepath.Clean(absPath), nil
}

func uniqueRootCompositionPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func rootCompositionPathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func rootCompositionSelectorsOverlap(left, right []string) bool {
	for _, leftSelector := range left {
		for _, rightSelector := range right {
			if rootCompositionPathWithin(leftSelector, rightSelector) || rootCompositionPathWithin(rightSelector, leftSelector) {
				return true
			}
		}
	}
	return false
}

type rootCompositionCommandRouter struct {
	registry *rootCompositionRouteRegistry
	kind     string
}

func (router *rootCompositionCommandRouter) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	if router.kind == "provider" {
		router.registry.providerCalls.Add(1)
	} else {
		router.registry.scriptCalls.Add(1)
	}
	route, err := router.registry.routeForCommand(request.WorkDir)
	if err != nil {
		return platformprocess.CommandResult{}, fmt.Errorf("%s command route rejected: %w", router.kind, err)
	}
	var runner platformprocess.CommandRunner
	if router.kind == "provider" {
		runner = route.providerRunner
	} else {
		runner = route.scriptRunner
	}
	if runner == nil {
		router.registry.unmatched.Add(1)
		return platformprocess.CommandResult{}, fmt.Errorf("%s command route %q has no runner", router.kind, route.label)
	}
	return runner.Run(ctx, request)
}

// rootCompositionProviderRouter is the one process-scoped Providers override
// installed in the shared root graph. It selects the scenario by the public
// Factory/working-directory identity carried by the request. Metadata methods
// use the inert NativeProvider contract; only Execute reaches a route-owned
// provider effect, and an execution with no route fails closed.
type rootCompositionProviderRouter struct {
	registry *rootCompositionRouteRegistry
}

func (router *rootCompositionProviderRouter) activeProvider() (providers.Service, error) {
	if router == nil || router.registry == nil {
		return nil, errors.New("shared provider route registry is unavailable")
	}
	return testutil.NativeProvider{}, nil
}

func (router *rootCompositionProviderRouter) providerForRequest(request providers.ExecuteRequest) (providers.Service, error) {
	path := strings.TrimSpace(request.FactoryDirectory)
	if path == "" {
		path = strings.TrimSpace(request.WorkingDirectory)
	}
	if path == "" {
		router.registry.unmatched.Add(1)
		return nil, fmt.Errorf("provider request has no route selector: %w", errRootCompositionRouteNotFound)
	}
	if route, err := router.registry.routeForEffectPath(path); err == nil && route.provider != nil {
		return route.provider, nil
	}
	if route, err := router.registry.routeForProvider(request); err == nil {
		return route.provider, nil
	}
	return nil, fmt.Errorf("no provider route for %q: %w", path, errRootCompositionRouteNotFound)
}

func (router *rootCompositionProviderRouter) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ListProvidersResult{}, err
	}
	return provider.ListProviders(ctx, request)
}

func (router *rootCompositionProviderRouter) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.GetProviderResult{}, err
	}
	return provider.GetProvider(ctx, request)
}

func (router *rootCompositionProviderRouter) ResolveIdentity(
	ctx context.Context,
	request providers.ResolveIdentityRequest,
) (providers.ResolveIdentityResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	return provider.ResolveIdentity(ctx, request)
}

func (router *rootCompositionProviderRouter) ResolveSelection(
	ctx context.Context,
	request providers.ResolveSelectionRequest,
) (providers.ResolveSelectionResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ResolveSelectionResult{}, err
	}
	return provider.ResolveSelection(ctx, request)
}

func (router *rootCompositionProviderRouter) ValidatePrerequisites(
	ctx context.Context,
	request providers.ValidatePrerequisitesRequest,
) error {
	provider, err := router.activeProvider()
	if err != nil {
		return err
	}
	return provider.ValidatePrerequisites(ctx, request)
}

func (router *rootCompositionProviderRouter) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	provider, err := router.providerForRequest(request)
	if err != nil {
		router.registry.unmatched.Add(1)
		return providers.ExecuteResult{}, fmt.Errorf("provider route rejected: %w", err)
	}
	result, err := provider.Execute(ctx, request)
	if err != nil {
		return result, observeRootCompositionProviderFailure(request, err)
	}
	observeRootCompositionProviderDiagnostics(request, result.Diagnostics)
	return result, nil
}

func observeRootCompositionProviderFailure(
	request providers.ExecuteRequest,
	err error,
) error {
	var failure providers.ExecuteFailure
	if errors.As(err, &failure) {
		failure = failure.Clone()
		observeRootCompositionProviderDiagnostics(request, failure.Diagnostics)
		return failure
	}
	var failurePointer *providers.ExecuteFailure
	if errors.As(err, &failurePointer) && failurePointer != nil {
		failure := failurePointer.Clone()
		observeRootCompositionProviderDiagnostics(request, failure.Diagnostics)
		return failure
	}
	return err
}

func observeRootCompositionProviderDiagnostics(
	request providers.ExecuteRequest,
	diagnostics *providers.ExecuteDiagnostics,
) {
	if diagnostics == nil || diagnostics.ProgressAlreadyObserved || request.ProgressObserver == nil {
		return
	}
	for _, progress := range diagnostics.Progress {
		request.ObserveProgress(progress)
	}
	diagnostics.ProgressAlreadyObserved = true
}

func (router *rootCompositionProviderRouter) ControlAttempt(
	ctx context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return provider.ControlAttempt(ctx, request)
}

func (router *rootCompositionProviderRouter) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return provider.Continue(ctx, request)
}

func (router *rootCompositionProviderRouter) ContinueReference(
	ctx context.Context,
	request providers.ContinueReferenceRequest,
) (providers.ContinueReferenceResult, error) {
	provider, err := router.activeProvider()
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	return provider.ContinueReference(ctx, request)
}

var _ providers.Service = (*rootCompositionProviderRouter)(nil)
