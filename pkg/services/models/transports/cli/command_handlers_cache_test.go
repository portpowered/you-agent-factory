package cli

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
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
