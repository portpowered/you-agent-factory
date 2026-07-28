package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	systeminitialization "github.com/portpowered/infinite-you/pkg/services/system_initialization"
	systeminitializationcli "github.com/portpowered/infinite-you/pkg/services/system_initialization/transports/cli"
)

func constructedBootstrapCLIService(
	t *testing.T,
	root *fakeBootstrapRoot,
) systeminitializationcli.Service {
	t.Helper()
	service := systeminitializationcli.New(root)
	if service == nil {
		t.Fatal("New(root) = nil, want Bootstrap CLI service")
	}
	return service
}

func TestConstructedService_InitializeHumanOutcomesMatchPackageCommand(t *testing.T) {
	t.Parallel()

	factoryDir := "/home/operator/.you-agent-factory/factories/@you/goal"
	configPath := "/home/operator/.you-agent-factory/config.json"
	namedFactoriesRoot := "/home/operator/.you-agent-factory/factories"
	cases := []struct {
		name                string
		systemConfigOutcome systeminitialization.SystemConfigOutcome
		factoryOutcome      systeminitialization.PackagedFactoryOutcome
		wantContains        []string
	}{
		{
			name:                "created",
			systemConfigOutcome: systeminitialization.SystemConfigCreated,
			factoryOutcome:      systeminitialization.PackagedFactoryCreated,
			wantContains: []string{
				"Initialized operator config at " + configPath,
				"Named factories root: " + namedFactoriesRoot,
				"Installed packaged factory @you/goal",
				factoryDir,
			},
		},
		{
			name:                "skipped",
			systemConfigOutcome: systeminitialization.SystemConfigSkipped,
			factoryOutcome:      systeminitialization.PackagedFactorySkipped,
			wantContains: []string{
				"Operator config already present at " + configPath,
				"Packaged factory @you/goal is already installed at " + factoryDir,
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &fakeBootstrapRoot{
				result: systeminitialization.Result{
					HomeDir:             "/home/operator",
					ConfigPath:          configPath,
					NamedFactoriesRoot:  namedFactoriesRoot,
					SystemConfigOutcome: tc.systemConfigOutcome,
					PackagedFactories: []systeminitialization.PackagedFactoryResult{
						{
							Name:       "@you/goal",
							FactoryDir: factoryDir,
							Outcome:    tc.factoryOutcome,
						},
					},
				},
			}
			service := constructedBootstrapCLIService(t, root)
			cfg := systeminitializationcli.InitializeConfig{
				Context: context.Background(),
				HomeDir: "/home/operator",
			}
			assertInitializeParity(t, service, root, cfg, nil)

			var output bytes.Buffer
			cfg.Output = &output
			if _, err := service.Initialize(cfg); err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			got := output.String()
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Fatalf("stdout = %q, want human output containing %q", got, want)
				}
			}
		})
	}
}

func TestConstructedService_InitializeJSONOutcomesMatchPackageCommand(t *testing.T) {
	t.Parallel()

	factoryDir := "/home/operator/.you-agent-factory/factories/@you/goal"
	configPath := "/home/operator/.you-agent-factory/config.json"
	namedFactoriesRoot := "/home/operator/.you-agent-factory/factories"
	cases := []struct {
		name                string
		systemConfigOutcome systeminitialization.SystemConfigOutcome
		factoryOutcome      systeminitialization.PackagedFactoryOutcome
	}{
		{
			name:                "created",
			systemConfigOutcome: systeminitialization.SystemConfigCreated,
			factoryOutcome:      systeminitialization.PackagedFactoryCreated,
		},
		{
			name:                "skipped",
			systemConfigOutcome: systeminitialization.SystemConfigSkipped,
			factoryOutcome:      systeminitialization.PackagedFactorySkipped,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &fakeBootstrapRoot{
				result: systeminitialization.Result{
					HomeDir:             "/home/operator",
					ConfigPath:          configPath,
					NamedFactoriesRoot:  namedFactoriesRoot,
					SystemConfigOutcome: tc.systemConfigOutcome,
					PackagedFactories: []systeminitialization.PackagedFactoryResult{
						{
							Name:       "@you/goal",
							FactoryDir: factoryDir,
							Outcome:    tc.factoryOutcome,
						},
					},
				},
			}
			service := constructedBootstrapCLIService(t, root)
			cfg := systeminitializationcli.InitializeConfig{
				Context: context.Background(),
				HomeDir: "/home/operator",
				JSON:    true,
			}
			assertInitializeParity(t, service, root, cfg, nil)

			var output bytes.Buffer
			cfg.Output = &output
			if _, err := service.Initialize(cfg); err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			var payload systeminitializationcli.InitializeResult
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if payload.HomeDir != "/home/operator" ||
				payload.ConfigPath != configPath ||
				payload.NamedFactoriesRoot != namedFactoriesRoot ||
				payload.SystemConfigOutcome != string(tc.systemConfigOutcome) {
				t.Fatalf("payload = %#v, want retained JSON fields", payload)
			}
			if len(payload.PackagedFactories) != 1 ||
				payload.PackagedFactories[0].Name != "@you/goal" ||
				payload.PackagedFactories[0].FactoryDir != factoryDir ||
				payload.PackagedFactories[0].Outcome != string(tc.factoryOutcome) {
				t.Fatalf("packaged factories = %#v, want retained JSON fields", payload.PackagedFactories)
			}
		})
	}
}

func TestConstructedService_InitializeValidationFailuresMatchPackageCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		cfg  systeminitializationcli.InitializeConfig
		want error
	}{
		{
			name: "missing home directory",
			cfg: systeminitializationcli.InitializeConfig{
				Context: context.Background(),
			},
			want: systeminitialization.ErrMissingHomeDir,
		},
		{
			name: "blank home directory",
			cfg: systeminitializationcli.InitializeConfig{
				Context: context.Background(),
				HomeDir: "   ",
			},
			want: systeminitialization.ErrMissingHomeDir,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &fakeBootstrapRoot{
				err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
			}
			service := constructedBootstrapCLIService(t, root)
			assertInitializeParity(t, service, root, tc.cfg, func() {
				var output bytes.Buffer
				cfg := tc.cfg
				cfg.Output = &output
				_, err := service.Initialize(cfg)
				if err == nil || !errors.Is(err, tc.want) {
					t.Fatalf("error = %v, want %v", err, tc.want)
				}
				if output.Len() != 0 {
					t.Fatalf("stdout = %q, want empty on validation failure", output.String())
				}
			})
		})
	}
}

func TestConstructedService_InitializeBootstrapErrorsMatchPackageCommand(t *testing.T) {
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
	cases := []struct {
		name      string
		root      *fakeBootstrapRoot
		wantIs    error
		wantWraps string
	}{
		{
			name: "missing home directory",
			root: &fakeBootstrapRoot{
				err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
			},
			wantIs: systeminitialization.ErrMissingHomeDir,
		},
		{
			name: "initialize cancelled",
			root: &fakeBootstrapRoot{
				err: fmt.Errorf("initialize system: %w: %w", systeminitialization.ErrInitializeCancelled, context.Canceled),
			},
			wantIs: systeminitialization.ErrInitializeCancelled,
		},
		{
			name:   "partial failure rollback facts",
			root:   &fakeBootstrapRoot{err: partialFailure},
			wantIs: systeminitialization.ErrInitializePartialFailure,
		},
		{
			name: "wrapped bootstrap rejection",
			root: &fakeBootstrapRoot{
				err: errors.New("bootstrap rejected"),
			},
			wantWraps: "initialize system: bootstrap rejected",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := constructedBootstrapCLIService(t, tc.root)
			cfg := systeminitializationcli.InitializeConfig{
				Context: context.Background(),
				HomeDir: "/home/operator",
			}
			assertInitializeParity(t, service, tc.root, cfg, func() {
				var output bytes.Buffer
				cfg.Output = &output
				_, err := service.Initialize(cfg)
				if err == nil {
					t.Fatal("Initialize() error = nil, want failure")
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Fatalf("error = %v, want %v", err, tc.wantIs)
				}
				if tc.wantWraps != "" && err.Error() != tc.wantWraps {
					t.Fatalf("error = %q, want %q", err.Error(), tc.wantWraps)
				}
				if tc.wantIs == systeminitialization.ErrInitializePartialFailure {
					var got systeminitialization.InitializePartialFailure
					if !errors.As(err, &got) {
						t.Fatalf("error = %T(%v), want InitializePartialFailure", err, err)
					}
					if len(got.Facts) != 2 || got.Facts[1].Outcome != systeminitialization.RollbackStepUnresolved {
						t.Fatalf("rollback facts = %#v, want inspectable partial-failure facts", got.Facts)
					}
				}
				if output.Len() != 0 {
					t.Fatalf("stdout = %q, want empty on failure", output.String())
				}
			})
		})
	}
}

func TestConstructedService_InitializeVerboseDiagnosticsMatchPackageCommand(t *testing.T) {
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
	service := constructedBootstrapCLIService(t, root)
	baseCfg := systeminitializationcli.InitializeConfig{
		Context: context.Background(),
		HomeDir: "/home/operator",
		Verbose: true,
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var serviceDiag, commandDiag bytes.Buffer
		var serviceOut, commandOut bytes.Buffer
		serviceCfg := baseCfg
		serviceCfg.Output = &serviceOut
		serviceCfg.Diagnostics = &serviceDiag
		if _, err := service.Initialize(serviceCfg); err != nil {
			t.Fatalf("service.Initialize() error = %v", err)
		}

		commandCfg := baseCfg
		commandCfg.Output = &commandOut
		commandCfg.Diagnostics = &commandDiag
		if _, err := systeminitializationcli.Initialize(commandCfg, root); err != nil {
			t.Fatalf("Initialize() error = %v", err)
		}
		if serviceOut.String() != commandOut.String() {
			t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
		}
		for _, want := range []string{
			`initialize system request homeDir="/home/operator"`,
			`initialize system complete homeDir="/home/operator" systemConfigOutcome=created packagedFactories=1`,
		} {
			if !strings.Contains(serviceDiag.String(), want) {
				t.Fatalf("service diagnostics missing %q:\n%s", want, serviceDiag.String())
			}
			if !strings.Contains(commandDiag.String(), want) {
				t.Fatalf("command diagnostics missing %q:\n%s", want, commandDiag.String())
			}
		}
	})

	t.Run("failure", func(t *testing.T) {
		t.Parallel()

		failingRoot := &fakeBootstrapRoot{
			err: fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
		}
		failingService := constructedBootstrapCLIService(t, failingRoot)

		var serviceDiag, commandDiag bytes.Buffer
		serviceCfg := baseCfg
		serviceCfg.Output = io.Discard
		serviceCfg.Diagnostics = &serviceDiag
		_, serviceErr := failingService.Initialize(serviceCfg)

		commandCfg := baseCfg
		commandCfg.Output = io.Discard
		commandCfg.Diagnostics = &commandDiag
		_, commandErr := systeminitializationcli.Initialize(commandCfg, failingRoot)

		if (serviceErr == nil) != (commandErr == nil) {
			t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
		}
		if serviceErr != nil && serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
		for _, want := range []string{
			`initialize system request homeDir="/home/operator"`,
			`initialize system failed homeDir="/home/operator"`,
		} {
			if !strings.Contains(serviceDiag.String(), want) {
				t.Fatalf("service diagnostics missing %q:\n%s", want, serviceDiag.String())
			}
			if !strings.Contains(commandDiag.String(), want) {
				t.Fatalf("command diagnostics missing %q:\n%s", want, commandDiag.String())
			}
		}
	})
}

func TestConstructedService_InitializeHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := &fakeBootstrapRoot{
		err: fmt.Errorf("initialize system: %w: %w", systeminitialization.ErrInitializeCancelled, context.Canceled),
	}
	service := constructedBootstrapCLIService(t, root)
	cfg := systeminitializationcli.InitializeConfig{
		Context: ctx,
		HomeDir: "/home/operator",
	}

	var serviceOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	_, serviceErr := service.Initialize(serviceCfg)
	if !errors.Is(serviceErr, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("service error = %v, want ErrInitializeCancelled", serviceErr)
	}
	if serviceOut.Len() != 0 {
		t.Fatalf("service output = %q, want empty on cancellation", serviceOut.String())
	}

	var commandOut bytes.Buffer
	commandCfg := cfg
	commandCfg.Output = &commandOut
	_, commandErr := systeminitializationcli.Initialize(commandCfg, root)
	if !errors.Is(commandErr, systeminitialization.ErrInitializeCancelled) {
		t.Fatalf("Initialize() error = %v, want ErrInitializeCancelled", commandErr)
	}
	if commandOut.Len() != 0 {
		t.Fatalf("command output = %q, want empty on cancellation", commandOut.String())
	}
}

func TestInitializeSystemPreservesExitErrorsOnFailurePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "missing home",
			err:  fmt.Errorf("%w", systeminitialization.ErrMissingHomeDir),
			want: systeminitialization.ErrMissingHomeDir,
		},
		{
			name: "cancelled",
			err:  fmt.Errorf("initialize system: %w: %w", systeminitialization.ErrInitializeCancelled, context.Canceled),
			want: systeminitialization.ErrInitializeCancelled,
		},
		{
			name: "partial failure",
			err: systeminitialization.InitializePartialFailure{
				Message: "packaged factory install failed",
			},
			want: systeminitialization.ErrInitializePartialFailure,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := &fakeBootstrapRoot{err: tc.err}
			err := systeminitializationcli.InitializeSystem(context.Background(), "/home/operator", root)
			if !errors.Is(err, tc.want) {
				t.Fatalf("InitializeSystem() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func assertInitializeParity(
	t *testing.T,
	service systeminitializationcli.Service,
	root *fakeBootstrapRoot,
	cfg systeminitializationcli.InitializeConfig,
	after func(),
) {
	t.Helper()

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
	if serviceErr != nil && commandErr != nil {
		if !errors.Is(serviceErr, commandErr) && serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
	}
	if serviceOut.String() != commandOut.String() {
		t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
	}
	if serviceErr == nil && serviceResult.HomeDir != commandResult.HomeDir {
		t.Fatalf("service result = %#v, command result = %#v", serviceResult, commandResult)
	}
	if cfg.JSON && serviceErr == nil {
		var payload systeminitializationcli.InitializeResult
		if err := json.Unmarshal(serviceOut.Bytes(), &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload.HomeDir == "" || payload.ConfigPath == "" || payload.NamedFactoriesRoot == "" {
			t.Fatalf("payload = %#v, want retained JSON fields", payload)
		}
	}
	if serviceErr == nil && !cfg.JSON && serviceOut.Len() > 0 {
		got := strings.ToLower(serviceOut.String())
		if !strings.Contains(got, "operator config") && !strings.Contains(got, "packaged factory") {
			t.Fatalf("stdout = %q, want human success output", serviceOut.String())
		}
	}
	if after != nil {
		after()
	}
}
