package runtimeapifixture

import (
	"context"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

type runtimeAPIProviderRouter struct {
	mu     sync.RWMutex
	routes map[string]runtimeAPIProviderRoute
	models map[string]string
}

type runtimeAPIProviderRoute struct {
	factoryDir string
	models     []string
	provider   providers.Service
	token      *struct{}
}

func newRuntimeAPIProviderRouter() *runtimeAPIProviderRouter {
	return &runtimeAPIProviderRouter{routes: make(map[string]runtimeAPIProviderRoute), models: make(map[string]string)}
}

func (router *runtimeAPIProviderRouter) register(factoryDir string, models []string, provider providers.Service) func() {
	key := runtimeAPINormalizeDir(factoryDir)
	route := runtimeAPIProviderRoute{
		factoryDir: key,
		models:     append([]string(nil), models...),
		provider:   provider,
		token:      &struct{}{},
	}
	router.mu.Lock()
	if previous, ok := router.routes[key]; ok {
		for _, model := range previous.models {
			modelKey := strings.ToLower(strings.TrimSpace(model))
			if router.models[modelKey] == key {
				delete(router.models, modelKey)
			}
		}
	}
	router.routes[key] = route
	for _, model := range models {
		router.models[strings.ToLower(strings.TrimSpace(model))] = key
	}
	router.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			router.mu.Lock()
			defer router.mu.Unlock()
			if current, ok := router.routes[key]; ok && current.token == route.token {
				delete(router.routes, key)
				for _, model := range route.models {
					modelKey := strings.ToLower(strings.TrimSpace(model))
					if router.models[modelKey] == key {
						delete(router.models, modelKey)
					}
				}
			}
		})
	}
}

func (router *runtimeAPIProviderRouter) routeCounts() (int, int) {
	if router == nil {
		return 0, 0
	}
	router.mu.RLock()
	defer router.mu.RUnlock()
	return len(router.routes), len(router.models)
}

func (router *runtimeAPIProviderRouter) providerFor(request providers.ExecuteRequest) providers.Service {
	factoryDir := runtimeAPINormalizeDir(request.FactoryDirectory)
	model := strings.ToLower(strings.TrimSpace(request.Model))
	router.mu.RLock()
	defer router.mu.RUnlock()
	if route, ok := router.routes[factoryDir]; ok {
		return route.provider
	}
	for _, route := range router.routes {
		if runtimeAPIDirContains(route.factoryDir, factoryDir) || runtimeAPIDirContains(route.factoryDir, runtimeAPINormalizeDir(request.WorkingDirectory)) {
			return route.provider
		}
	}
	if key := router.models[model]; key != "" {
		return router.routes[key].provider
	}
	return nil
}

func (router *runtimeAPIProviderRouter) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	provider := router.providerFor(request)
	if provider == nil {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindMisconfigured,
			Message: "no shared runtime API provider route for factory directory",
		}
	}
	return provider.Execute(ctx, request)
}

func (router *runtimeAPIProviderRouter) ListProviders(ctx context.Context, request providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return (testutil.NativeProvider{}).ListProviders(ctx, request)
}

func (router *runtimeAPIProviderRouter) GetProvider(ctx context.Context, request providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return (testutil.NativeProvider{}).GetProvider(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveIdentity(ctx context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	return (testutil.NativeProvider{}).ResolveIdentity(ctx, request)
}

func (router *runtimeAPIProviderRouter) ResolveSelection(ctx context.Context, request providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return (testutil.NativeProvider{}).ResolveSelection(ctx, request)
}

func (router *runtimeAPIProviderRouter) ValidatePrerequisites(ctx context.Context, request providers.ValidatePrerequisitesRequest) error {
	return (testutil.NativeProvider{}).ValidatePrerequisites(ctx, request)
}

func (router *runtimeAPIProviderRouter) ControlAttempt(ctx context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	return (testutil.NativeProvider{}).ControlAttempt(ctx, request)
}

func (router *runtimeAPIProviderRouter) Continue(ctx context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	return (testutil.NativeProvider{}).Continue(ctx, request)
}

func (router *runtimeAPIProviderRouter) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	return (testutil.NativeProvider{}).ContinueReference(ctx, request)
}

var _ providers.Service = (*runtimeAPIProviderRouter)(nil)
