package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationcli "github.com/portpowered/infinite-you/pkg/services/system_initialization/transports/cli"
)

func TestNewRequiresBootstrapRoot(t *testing.T) {
	t.Parallel()

	if service := systeminitializationcli.New(nil); service != nil {
		t.Fatalf("New(nil) = %T, want nil", service)
	}
}

func TestConstructedService_InitializeCreatedPath(t *testing.T) {
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
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	result, err := service.Initialize(systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
	})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigCreated {
		t.Fatalf("result.SystemConfigOutcome = %q, want created", result.SystemConfigOutcome)
	}
	if root.request.HomeDir != "/home/operator" {
		t.Fatalf("request.HomeDir = %q, want /home/operator", root.request.HomeDir)
	}
}

func TestConstructedService_InitializeSkippedPath(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		result: systeminitialization.Result{
			HomeDir:             "/home/operator",
			ConfigPath:          "/home/operator/.you-agent-factory/config.json",
			NamedFactoriesRoot:  "/home/operator/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigSkipped,
			PackagedFactories: []systeminitialization.PackagedFactoryResult{
				{
					Name:       "@you/goal",
					FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
					Outcome:    systeminitialization.PackagedFactorySkipped,
				},
			},
		},
	}
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	result, err := service.Initialize(systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
	})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.SystemConfigOutcome != systeminitialization.SystemConfigSkipped {
		t.Fatalf("result.SystemConfigOutcome = %q, want skipped", result.SystemConfigOutcome)
	}
	if result.PackagedFactories[0].Outcome != systeminitialization.PackagedFactorySkipped {
		t.Fatalf("packaged factory outcome = %q, want skipped", result.PackagedFactories[0].Outcome)
	}
}

func TestConstructedService_InitializeMissingHomeDir(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
	}
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	_, err := service.Initialize(systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "",
	})
	if !errors.Is(err, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("Initialize() error = %v, want ErrMissingHomeDir", err)
	}
}

func TestConstructedService_InitializeCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := &fakeBootstrapRoot{
		err: fmt.Errorf("initialize system: %w: %w", systeminitialization.ErrInitializeCancelled, context.Canceled),
	}
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	_, err := service.Initialize(systeminitializationcli.InitializeConfig{
		Context: ctx,
		HomeDir: "/home/operator",
	})
	if !errors.Is(err, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("Initialize() error = %v, want ErrInitializeCancelled", err)
	}
}

func TestConstructedService_InitializePartialFailureRollbackFacts(t *testing.T) {
	t.Parallel()

	partialFailure := systeminitialization.InitializePartialFailure{
		Message: "packaged factory install failed",
		Facts: []systeminitialization.RollbackFact{
			{
				Step:    systeminitialization.InitializeStepSystemConfig,
				Outcome: systeminitialization.RollbackStepCompleted,
			},
			{
				Step:    systeminitialization.InitializeStepPackagedFactories,
				Outcome: systeminitialization.RollbackStepUnresolved,
			},
		},
	}
	root := &fakeBootstrapRoot{err: partialFailure}
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	_, err := service.Initialize(systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
	})
	if !errors.Is(err, systeminitialization.ErrInitializePartialFailure) {
		t.Fatalf("Initialize() error = %v, want ErrInitializePartialFailure", err)
	}
	var got systeminitialization.InitializePartialFailure
	if !errors.As(err, &got) {
		t.Fatalf("Initialize() error = %T(%v), want InitializePartialFailure", err, err)
	}
	if len(got.Facts) != 2 || got.Facts[1].Outcome != systeminitialization.RollbackStepUnresolved {
		t.Fatalf("rollback facts = %#v, want inspectable partial-failure facts", got.Facts)
	}
}

func TestInitializeFacadeMatchesConstructedService(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		result: systeminitialization.Result{
			HomeDir:             "/home/operator",
			ConfigPath:          "/home/operator/.you-agent-factory/config.json",
			NamedFactoriesRoot:  "/home/operator/.you-agent-factory/factories",
			SystemConfigOutcome: systeminitialization.SystemConfigCreated,
		},
	}
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}

	cfg := systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
		JSON:    true,
	}
	var serviceOut, commandOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	commandCfg := cfg
	commandCfg.Output = &commandOut

	serviceResult, serviceErr := service.Initialize(serviceCfg)
	commandResult, commandErr := systeminitializationcli.Initialize(commandCfg, root)
	if (serviceErr == nil) != (commandErr == nil) {
		t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
	if serviceResult.HomeDir != commandResult.HomeDir {
		t.Fatalf("service result = %#v, command result = %#v", serviceResult, commandResult)
	}
	if serviceErr == nil {
		var payload systeminitializationcli.InitializeResult
		if err := json.Unmarshal(serviceOut.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload.HomeDir == "" || payload.ConfigPath == "" || payload.NamedFactoriesRoot == "" {
			t.Fatalf("payload = %#v, want retained JSON fields", payload)
		}
	}
}

func TestInitializeSystemFacadePreservesExitErrors(t *testing.T) {
	t.Parallel()

	root := &fakeBootstrapRoot{
		err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
	}
	err := systeminitializationcli.InitializeSystem(context.Background(), "", root)
	if !errors.Is(err, systeminitialization.ErrMissingHomeDir) {
		t.Fatalf("InitializeSystem() error = %v, want ErrMissingHomeDir", err)
	}
}

type fakeBootstrapRoot struct {
	request systeminitialization.Request
	result  systeminitialization.Result
	err     error
}

func (fake *fakeBootstrapRoot) Initialize(
	_ context.Context,
	request systeminitialization.Request,
) (systeminitialization.Result, error) {
	fake.request = request
	if fake.err != nil {
		return systeminitialization.Result{}, fake.err
	}
	return fake.result, nil
}
