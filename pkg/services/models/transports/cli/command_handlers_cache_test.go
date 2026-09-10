package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

type cacheAwareCommandServiceFake struct {
	commandServiceFake
	invokeWithModelCache func(InvokeConfig, string) error
}

func (fake cacheAwareCommandServiceFake) InvokeWithModelCache(cfg InvokeConfig, modelCacheDir string) error {
	if fake.invokeWithModelCache == nil {
		return errors.New("unexpected invocation-local model cache call")
	}
	return fake.invokeWithModelCache(cfg, modelCacheDir)
}

type cacheAwareCatalogCommandServiceFake struct {
	commandServiceFake
	listWithModelCache    func(ListConfig, string) error
	inspectWithModelCache func(InspectConfig, string) error
	pullWithModelCache    func(PullConfig, string) error
	removeWithModelCache  func(RemoveConfig, string) error
}

func (fake cacheAwareCatalogCommandServiceFake) ListWithModelCache(cfg ListConfig, cache string) error {
	return fake.listWithModelCache(cfg, cache)
}

func (fake cacheAwareCatalogCommandServiceFake) InspectWithModelCache(cfg InspectConfig, cache string) error {
	return fake.inspectWithModelCache(cfg, cache)
}

func (fake cacheAwareCatalogCommandServiceFake) PullWithModelCache(cfg PullConfig, cache string) error {
	return fake.pullWithModelCache(cfg, cache)
}

func (fake cacheAwareCatalogCommandServiceFake) RemoveWithModelCache(cfg RemoveConfig, cache string) error {
	return fake.removeWithModelCache(cfg, cache)
}

func TestCommandHandlerCatalogCommandsSelectOneResolvedCache(t *testing.T) {
	t.Parallel()

	const selectedCache = "/selected/model-cache"
	calls := map[string]string{}
	service := cacheAwareCatalogCommandServiceFake{
		listWithModelCache: func(_ ListConfig, cache string) error {
			calls["list"] = cache
			return nil
		},
		inspectWithModelCache: func(_ InspectConfig, cache string) error {
			calls["inspect"] = cache
			return nil
		},
		pullWithModelCache: func(_ PullConfig, cache string) error {
			calls["pull"] = cache
			return nil
		},
		removeWithModelCache: func(_ RemoveConfig, cache string) error {
			calls["remove"] = cache
			return nil
		},
	}
	resolverCalls := 0
	handler := NewCommandHandler(
		service,
		nil,
		nil,
		nil,
		nil,
		func() (string, error) {
			resolverCalls++
			return selectedCache, nil
		},
	)
	cmd := &cobra.Command{Use: "models"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	inspectInputs, pullInputs, inherited := resolvedModelsHandlerInputs(t, "")
	if err := handler.List(cmd, resolvedinput.Inputs{}, inherited); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if err := handler.Inspect(cmd, inspectInputs, inherited); err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if err := handler.Pull(cmd, pullInputs, inherited); err != nil {
		t.Fatalf("Pull() error = %v", err)
	}
	removeInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{
			ID: modelsRemoveNameInputID, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
		}},
		[]resolvedinput.Candidate{{
			InputID: modelsRemoveNameInputID, Source: resolvedinput.SourcePositionalArgument,
			Value: resolvedinput.StringValue("model-a"),
		}},
	)
	if err != nil {
		t.Fatalf("resolve remove inputs: %v", err)
	}
	if err := handler.Remove(cmd, removeInputs, inherited); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if resolverCalls != 4 {
		t.Fatalf("model cache resolver calls = %d, want 4", resolverCalls)
	}
	for _, operation := range []string{"list", "inspect", "pull", "remove"} {
		if calls[operation] != selectedCache {
			t.Fatalf("%s selected cache = %q, want %q", operation, calls[operation], selectedCache)
		}
	}
}

func TestCommandHandlerCatalogCommandsPreserveLegacyForEmptyAndServerSelection(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name   string
		server string
		resolv string
		want   int
	}{
		{name: "empty selection", server: "", resolv: "", want: 4},
		{name: "explicit server", server: "http://127.0.0.1:7437", resolv: "/selected/model-cache", want: 0},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			legacyCalls := 0
			selectedCalls := 0
			legacy := func() error {
				legacyCalls++
				return nil
			}
			selected := func(ListConfig, string) error {
				selectedCalls++
				return nil
			}
			service := cacheAwareCatalogCommandServiceFake{
				commandServiceFake: commandServiceFake{
					list:    func(ListConfig) error { return legacy() },
					inspect: func(InspectConfig) error { return legacy() },
					pull:    func(PullConfig) error { return legacy() },
					remove:  func(RemoveConfig) error { return legacy() },
				},
				listWithModelCache: func(cfg ListConfig, cache string) error {
					return selected(cfg, cache)
				},
				inspectWithModelCache: func(cfg InspectConfig, cache string) error {
					selectedCalls++
					return nil
				},
				pullWithModelCache: func(cfg PullConfig, cache string) error {
					selectedCalls++
					return nil
				},
				removeWithModelCache: func(cfg RemoveConfig, cache string) error {
					selectedCalls++
					return nil
				},
			}
			resolverCalls := 0
			handler := NewCommandHandler(service, nil, nil, nil, nil, func() (string, error) {
				resolverCalls++
				return testCase.resolv, nil
			})
			cmd := &cobra.Command{Use: "models"}
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			inspectInputs, pullInputs, inherited := resolvedModelsHandlerInputs(t, testCase.server)
			if err := handler.List(cmd, resolvedinput.Inputs{}, inherited); err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if err := handler.Inspect(cmd, inspectInputs, inherited); err != nil {
				t.Fatalf("Inspect() error = %v", err)
			}
			if err := handler.Pull(cmd, pullInputs, inherited); err != nil {
				t.Fatalf("Pull() error = %v", err)
			}
			removeInputs, err := resolvedinput.Resolve(
				[]resolvedinput.Definition{{
					ID: modelsRemoveNameInputID, Kind: resolvedinput.ValueKindString,
					Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument},
				}},
				[]resolvedinput.Candidate{{
					InputID: modelsRemoveNameInputID, Source: resolvedinput.SourcePositionalArgument,
					Value: resolvedinput.StringValue("model-a"),
				}},
			)
			if err != nil {
				t.Fatalf("resolve remove inputs: %v", err)
			}
			if err := handler.Remove(cmd, removeInputs, inherited); err != nil {
				t.Fatalf("Remove() error = %v", err)
			}
			if legacyCalls != 4 || selectedCalls != 0 || resolverCalls != testCase.want {
				t.Fatalf("catalog fallback calls = legacy:%d selected:%d resolver:%d, want legacy:4 selected:0 resolver:%d", legacyCalls, selectedCalls, resolverCalls, testCase.want)
			}
		})
	}
}

func TestCommandHandlerCatalogCacheResolverFailureShortCircuitsModels(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("cache selection unavailable")
	serviceCalls := 0
	handler := NewCommandHandler(
		commandServiceFake{list: func(ListConfig) error {
			serviceCalls++
			return nil
		}},
		nil, nil, nil, nil,
		func() (string, error) { return "", resolverErr },
	)
	cmd := &cobra.Command{Use: "models"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	_, _, inherited := resolvedModelsHandlerInputs(t, "")
	err := handler.List(cmd, resolvedinput.Inputs{}, inherited)
	if !errors.Is(err, resolverErr) || !strings.Contains(err.Error(), "resolve model cache directory") {
		t.Fatalf("List() error = %v, want wrapped cache resolver error", err)
	}
	if serviceCalls != 0 {
		t.Fatalf("Models service calls = %d, want zero after resolver failure", serviceCalls)
	}
}

func TestCommandHandlerCatalogCacheSelectionFailsClosedWithoutOptionalService(t *testing.T) {
	t.Parallel()

	serviceCalls := 0
	handler := NewCommandHandler(
		commandServiceFake{list: func(ListConfig) error {
			serviceCalls++
			return nil
		}},
		nil, nil, nil, nil,
		func() (string, error) { return "/selected/model-cache", nil },
	)
	cmd := &cobra.Command{Use: "models"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	_, _, inherited := resolvedModelsHandlerInputs(t, "")
	err := handler.List(cmd, resolvedinput.Inputs{}, inherited)
	if err == nil || !strings.Contains(err.Error(), "does not support local model cache selection") {
		t.Fatalf("List() error = %v, want fail-closed optional-capability error", err)
	}
	if serviceCalls != 0 {
		t.Fatalf("Models service calls = %d, want zero without optional capability", serviceCalls)
	}
}

func TestCommandHandlerInvokeWithCacheResolverPreservesLegacyConfigAndSelectsCache(t *testing.T) {
	t.Parallel()

	const selectedCache = "/selected/model-cache"
	server := "http://127.0.0.1:7437"
	logger := zap.NewNop()
	var received InvokeConfig
	var receivedCache string
	resolverCalls := 0
	var diagnostics io.Writer = io.Discard

	service := cacheAwareCommandServiceFake{
		commandServiceFake: commandServiceFake{},
		invokeWithModelCache: func(cfg InvokeConfig, modelCacheDir string) error {
			received = cfg
			receivedCache = modelCacheDir
			return nil
		},
	}
	handler := NewCommandHandler(
		service,
		func(*cobra.Command) io.Writer { return diagnostics },
		func() (string, error) { return "/home/tester", nil },
		func(_ *cobra.Command, homeDir string) (operatorconfig.ResolvedDefaults, error) {
			if homeDir != "/home/tester" {
				t.Fatalf("operator defaults home = %q", homeDir)
			}
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return logger, nil },
		func() (string, error) {
			resolverCalls++
			return selectedCache, nil
		},
	)

	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	invokeInputs, inherited := resolvedInvokeHandlerInputs(t, server)
	if err := handler.Invoke(cmd, invokeInputs, inherited); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("model cache resolver calls = %d, want 1", resolverCalls)
	}
	if receivedCache != selectedCache {
		t.Fatalf("selected model cache = %q, want %q", receivedCache, selectedCache)
	}
	assertInvokeCommandConfig(t, received, server, logger, diagnostics)
}

func TestCommandHandlerInvokeWithoutCacheResolverPreservesLegacyServiceCall(t *testing.T) {
	t.Parallel()

	var received InvokeConfig
	handler := NewCommandHandler(
		commandServiceFake{invoke: func(cfg InvokeConfig) error {
			received = cfg
			return nil
		}},
		nil,
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
	)
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	inputs, inherited := resolvedInvokeHandlerInputs(t, "")
	if err := handler.Invoke(cmd, inputs, inherited); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if received.Context == nil || received.HomeDir != "/home/tester" {
		t.Fatalf("legacy InvokeConfig = %#v, want existing request values", received)
	}
}

func TestCommandHandlerInvokeEmptyCacheResolverPreservesLegacyServiceCall(t *testing.T) {
	t.Parallel()

	resolverCalls := 0
	var received InvokeConfig
	handler := NewCommandHandler(
		commandServiceFake{invoke: func(cfg InvokeConfig) error {
			received = cfg
			return nil
		}},
		nil,
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
		func() (string, error) {
			resolverCalls++
			return "", nil
		},
	)
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	inputs, inherited := resolvedInvokeHandlerInputs(t, "")
	if err := handler.Invoke(cmd, inputs, inherited); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("model cache resolver calls = %d, want 1", resolverCalls)
	}
	if received.Context == nil {
		t.Fatal("legacy service did not receive InvokeConfig after empty cache resolution")
	}
}

func TestCommandHandlerInvokeCacheResolverFailureShortCircuitsModels(t *testing.T) {
	t.Parallel()

	resolverErr := errors.New("cache selection unavailable")
	resolverCalls := 0
	serviceCalls := 0
	handler := NewCommandHandler(
		commandServiceFake{invoke: func(InvokeConfig) error {
			serviceCalls++
			return nil
		}},
		nil,
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
		func() (string, error) {
			resolverCalls++
			return "", resolverErr
		},
	)
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	cmd.SetOut(io.Discard)
	inputs, inherited := resolvedInvokeHandlerInputs(t, "")
	err := handler.Invoke(cmd, inputs, inherited)
	if !errors.Is(err, resolverErr) || !strings.Contains(err.Error(), "resolve model cache directory") {
		t.Fatalf("Invoke() error = %v, want wrapped cache resolver error", err)
	}
	if resolverCalls != 1 {
		t.Fatalf("model cache resolver calls = %d, want 1", resolverCalls)
	}
	if serviceCalls != 0 {
		t.Fatalf("Models service calls = %d, want 0 after resolver failure", serviceCalls)
	}
}

// legacyPositionalInvokeConfig is a compile-time compatibility check for the
// exported request shape used by existing embedded callers. Keep this literal
// in the historical field order when changing InvokeConfig.
func legacyPositionalInvokeConfig() InvokeConfig {
	return InvokeConfig{
		context.Background(), "", "", "", nil, nil, nil, "", nil,
		"", "", "", "", operatorconfig.ResolvedDefaults{}, nil,
		false, false, false, io.Discard, io.Discard,
	}
}

func TestInvokeConfigRetainsLegacyPositionalShape(t *testing.T) {
	t.Parallel()

	_ = legacyPositionalInvokeConfig()
}
