package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationcli "github.com/portpowered/infinite-you/pkg/services/system_initialization/transports/cli"
)

func TestBindInitializeSystemRequiresBootstrapRoot(t *testing.T) {
	t.Parallel()

	if operation := systeminitializationcli.BindInitializeSystem(nil); operation != nil {
		t.Fatalf("BindInitializeSystem(nil) = %T, want nil", operation)
	}
}

func TestBindInitializeRequiresBootstrapRoot(t *testing.T) {
	t.Parallel()

	if operation := systeminitializationcli.BindInitialize(nil); operation != nil {
		t.Fatalf("BindInitialize(nil) = %T, want nil", operation)
	}
}

func TestBindInitializeSystemDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		result: systeminitialization.Result{
			HomeDir:             "/home/operator",
			ConfigPath:          "/home/operator/.you-agent-factory/config.json",
			NamedFactoriesRoot:  "/home/operator/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
		},
	}
	operation := systeminitializationcli.BindInitializeSystem(root)
	if operation == nil {
		t.Fatal("BindInitializeSystem(root) = nil, want composition operation")
	}

	ctx := context.WithValue(context.Background(), contextKey("invocation"), "run")
	if err := operation(ctx, "/home/operator"); err != nil {
		t.Fatalf("operation(ctx, homeDir) error = %v", err)
	}
	if root.request.HomeDir != "/home/operator" {
		t.Fatalf("request.HomeDir = %q, want /home/operator", root.request.HomeDir)
	}
}

func TestBindInitializeDelegatesThroughAdapterService(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		result: systeminitialization.Result{
			HomeDir:             "/home/operator",
			ConfigPath:          "/home/operator/.you-agent-factory/config.json",
			NamedFactoriesRoot:  "/home/operator/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
			PackagedFactories: []systeminitialization.PackagedFactoryResult{
				{
					Name:       "@you/goal",
					FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
					Outcome:    systeminitialization.PackagedFactoryCreated,
				},
			},
		},
	}
	operation := systeminitializationcli.BindInitialize(root)
	if operation == nil {
		t.Fatal("BindInitialize(root) = nil, want composition operation")
	}

	var output bytes.Buffer
	cfg := systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
		Output:  &output,
	}
	result, err := operation(cfg)
	if err != nil {
		t.Fatalf("operation(cfg) error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("result.SystemConfigOutcome = %q, want created", result.SystemConfigOutcome)
	}
	if root.request.HomeDir != "/home/operator" {
		t.Fatalf("request.HomeDir = %q, want /home/operator", root.request.HomeDir)
	}
	if output.String() == "" {
		t.Fatal("operation output is empty, want human success presentation")
	}
}

func TestBindInitializeSystemMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
	}
	operation := systeminitializationcli.BindInitializeSystem(root)

	ctx := context.Background()
	boundErr := operation(ctx, "")
	directErr := systeminitializationcli.InitializeSystem(ctx, "", root)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && !errors.Is(boundErr, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("bound error = %v, want ErrMissingHomeDir", boundErr)
	}
}

func TestBindInitializeMatchesFreeFunctionFacade(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
	}
	operation := systeminitializationcli.BindInitialize(root)

	cfg := systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "",
	}
	boundResult, boundErr := operation(cfg)
	_, directErr := systeminitializationcli.Initialize(cfg, root)
	if (boundErr == nil) != (directErr == nil) {
		t.Fatalf("bound error = %v, direct error = %v", boundErr, directErr)
	}
	if boundErr != nil && !errors.Is(boundErr, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("bound error = %v, want ErrMissingHomeDir", boundErr)
	}
	if boundResult.HomeDir != "" || len(boundResult.PackagedFactories) != 0 {
		t.Fatalf("bound result = %#v, want zero result on failure", boundResult)
	}
}

func TestBindInitializeSystemRoutesThroughInitializerCompositionPath(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		result: systeminitialization.Result{
			HomeDir:             "customer-home",
			ConfigPath:          "customer-home/.you-agent-factory/config.json",
			NamedFactoriesRoot:  "customer-home/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
		},
	}
	initializer, err := initializerapplication.NewInitializer(
		&compositionStdioOpener{},
		systeminitializationcli.BindInitializeSystem(root),
	)
	if err != nil {
		t.Fatalf("NewInitializer() error = %v", err)
	}

	ctx := context.WithValue(context.Background(), contextKey("invocation"), "mcp-serve")
	if err := initializer.InitializeSystem(ctx, "customer-home"); err != nil {
		t.Fatalf("InitializeSystem() error = %v", err)
	}
	if root.request.HomeDir != "customer-home" {
		t.Fatalf("request.HomeDir = %q, want customer-home", root.request.HomeDir)
	}
}

type contextKey string

type compositionStdioOpener struct{}

func (compositionStdioOpener) OpenStdio(
	context.Context,
	startupcli.MCPIntent,
) (initializer.RunApplication, error) {
	panic("composition test does not open stdio")
}
