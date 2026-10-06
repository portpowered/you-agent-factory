package runtime_api

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"strings"
	"sync"
)

func twoStagePipelineConfig() map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "stage1", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{
			{
				"name":          "worker-a",
				"type":          "MODEL_WORKER",
				"model":         "functional-model",
				"modelProvider": "CODEX",
			},
			{
				"name":          "worker-b",
				"type":          "MODEL_WORKER",
				"model":         "functional-model",
				"modelProvider": "CODEX",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "worker-a",
				"worker":    "worker-a",
				"type":      "MODEL_WORKSTATION",
				"behavior":  "STANDARD",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "stage1"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":      "worker-b",
				"worker":    "worker-b",
				"type":      "MODEL_WORKSTATION",
				"behavior":  "STANDARD",
				"inputs":    []map[string]string{{"workType": "task", "state": "stage1"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}

type runtimeAPIProviderRouter struct {
	testutil.NativeProvider
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

type runtimeAPICommandProvider struct {
	testutil.NativeProvider
	runner platformprocess.CommandRunner
}

func newRuntimeAPICommandProvider(runner platformprocess.CommandRunner) providers.Service {
	provider := &runtimeAPICommandProvider{runner: runner}
	provider.NativeProvider.ExecuteFunc = provider.Execute
	return provider
}

func (provider *runtimeAPICommandProvider) Execute(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
	command := strings.TrimSpace(request.Command)
	if command == "" {
		command = request.Provider.CanonicalSessionProvider()
	}
	commandRequest := platformprocess.CommandRequest{
		Command:                  command,
		Args:                     append([]string(nil), request.Args...),
		Stdin:                    []byte(request.UserMessage),
		Env:                      append([]string(nil), request.ProcessEnvironment...),
		WorkDir:                  request.WorkingDirectory,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	result, err := provider.runner.Run(ctx, commandRequest)
	if err != nil {
		return providers.ExecuteResult{}, err
	}
	if failure := runtimeAPICommandFailure(result); failure != nil {
		return providers.ExecuteResult{}, failure
	}
	return providers.ExecuteResult{Content: runtimeAPICommandContent(command, result.Stdout)}, nil
}

func runtimeAPICommandFailure(result platformprocess.CommandResult) error {
	if result.ExitCode == 0 && len(result.Stderr) == 0 {
		return nil
	}
	message := strings.TrimSpace(string(result.Stderr))
	lower := strings.ToLower(message)
	kind := providers.ExecuteFailureKindUnknown
	switch {
	case strings.Contains(lower, "rate_limit"), strings.Contains(lower, "429"), strings.Contains(lower, "thrott"):
		kind = providers.ExecuteFailureKindThrottled
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "401"):
		kind = providers.ExecuteFailureKindAuthentication
	case strings.Contains(lower, "invalid"):
		kind = providers.ExecuteFailureKindInvalidRequest
	}
	if message == "" {
		message = "provider command failed"
	}
	return providers.ExecuteFailure{Kind: kind, Message: message}
}

func runtimeAPICommandContent(command string, stdout []byte) string {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record map[string]any
		if json.Unmarshal([]byte(line), &record) != nil {
			continue
		}
		typeName, _ := record["type"].(string)
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "codex":
			if typeName != "item.completed" {
				continue
			}
			item, _ := record["item"].(map[string]any)
			text, _ := item["text"].(string)
			if text != "" {
				return text
			}
		case "claude":
			if typeName != "result" {
				continue
			}
			text, _ := record["result"].(string)
			if text != "" {
				return text
			}
		}
	}
	return trimmed
}

var _ providers.Service = (*runtimeAPICommandProvider)(nil)
