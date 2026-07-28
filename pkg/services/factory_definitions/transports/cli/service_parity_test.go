package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionscli "github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/cli"
)

func constructedDefinitionsCLIService(
	t *testing.T,
	install factorydefinitionscli.InstallPackagedFactoryOperation,
) factorydefinitionscli.Service {
	t.Helper()
	service := factorydefinitionscli.New(install)
	if service == nil {
		t.Fatal("New(install) = nil, want Definitions CLI service")
	}
	return service
}

func TestConstructedService_InstallPackagedFactoryHumanOutcomesMatchPackageCommand(t *testing.T) {
	t.Parallel()

	factoryDir := "/home/operator/.you-agent-factory/factories/@you/goal"
	cases := []struct {
		name    string
		outcome factorydefinitions.PackagedFactoryInstallOutcome
		want    string
	}{
		{
			name:    "created",
			outcome: factorydefinitions.PackagedFactoryInstallCreated,
			want:    "Installed packaged factory @you/goal",
		},
		{
			name:    "skipped",
			outcome: factorydefinitions.PackagedFactoryInstallSkipped,
			want:    "Packaged factory @you/goal is already installed at " + factoryDir,
		},
		{
			name:    "replaced",
			outcome: factorydefinitions.PackagedFactoryInstallReplaced,
			want:    "Replaced packaged factory @you/goal",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			install := successInstallOperation(tc.outcome)
			service := constructedDefinitionsCLIService(t, install)
			cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
				Context: ctx(t),
				HomeDir: "/home/operator",
				Package: "@you/goal",
			}
			assertInstallPackagedFactoryParity(t, service, install, cfg, func() {
				// parity assertion handled by output comparison
			})
			var output bytes.Buffer
			cfg.Output = &output
			if err := service.InstallPackagedFactory(cfg); err != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", err)
			}
			if got := output.String(); !strings.Contains(got, tc.want) {
				t.Fatalf("stdout = %q, want human output containing %q", got, tc.want)
			}
			if !strings.Contains(output.String(), factoryDir) {
				t.Fatalf("stdout = %q, want factoryDir %q", output.String(), factoryDir)
			}
		})
	}
}

func TestConstructedService_InstallPackagedFactoryJSONOutcomesMatchPackageCommand(t *testing.T) {
	t.Parallel()

	factoryDir := "/home/operator/.you-agent-factory/factories/@you/goal"
	cases := []struct {
		name    string
		outcome factorydefinitions.PackagedFactoryInstallOutcome
		format  factorydefinitions.PackagedFactoryFormat
	}{
		{
			name:    "created",
			outcome: factorydefinitions.PackagedFactoryInstallCreated,
			format:  factorydefinitions.PackagedFactoryFormatJSON,
		},
		{
			name:    "skipped",
			outcome: factorydefinitions.PackagedFactoryInstallSkipped,
			format:  factorydefinitions.PackagedFactoryFormatJSON,
		},
		{
			name:    "replaced",
			outcome: factorydefinitions.PackagedFactoryInstallReplaced,
			format:  factorydefinitions.PackagedFactoryFormatYAML,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			install := successInstallOperation(tc.outcome, tc.format)
			service := constructedDefinitionsCLIService(t, install)
			cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
				Context: ctx(t),
				HomeDir: "/home/operator",
				Package: "@you/goal",
				JSON:    true,
			}
			assertInstallPackagedFactoryParity(t, service, install, cfg, nil)

			var output bytes.Buffer
			cfg.Output = &output
			if err := service.InstallPackagedFactory(cfg); err != nil {
				t.Fatalf("InstallPackagedFactory() error = %v", err)
			}
			var payload factorydefinitionscli.InstallPackagedFactoryResult
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if payload.Name != "@you/goal" ||
				payload.FactoryDir != factoryDir ||
				payload.Outcome != string(tc.outcome) ||
				payload.Format != string(tc.format) {
				t.Fatalf("payload = %#v, want retained JSON fields", payload)
			}
		})
	}
}

func TestConstructedService_InstallPackagedFactoryValidationFailuresMatchPackageCommand(t *testing.T) {
	t.Parallel()

	install := func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		t.Fatal("install operation should not run for validation failures")
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	}
	service := constructedDefinitionsCLIService(t, install)

	cases := []struct {
		name string
		cfg  factorydefinitionscli.InstallPackagedFactoryConfig
		want string
	}{
		{
			name: "missing package",
			cfg: factorydefinitionscli.InstallPackagedFactoryConfig{
				Context: ctx(t),
				HomeDir: "/home/operator",
			},
			want: "package name is required",
		},
		{
			name: "missing home directory",
			cfg: factorydefinitionscli.InstallPackagedFactoryConfig{
				Context: ctx(t),
				Package: "@you/goal",
			},
			want: "home directory is required",
		},
		{
			name: "empty destination directory",
			cfg: factorydefinitionscli.InstallPackagedFactoryConfig{
				Context:    ctx(t),
				HomeDir:    "/home/operator",
				Package:    "@you/goal",
				Dir:        "   ",
				DirChanged: true,
			},
			want: "destination directory must be non-empty",
		},
		{
			name: "relative destination without working directory",
			cfg: factorydefinitionscli.InstallPackagedFactoryConfig{
				Context:    context.Background(),
				HomeDir:    "/home/operator",
				Package:    "@you/goal",
				Dir:        "alternate-factories",
				DirChanged: true,
			},
			want: "process working directory is required for relative destination",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertInstallPackagedFactoryParity(t, service, install, tc.cfg, func() {
				var output bytes.Buffer
				cfg := tc.cfg
				cfg.Output = &output
				err := service.InstallPackagedFactory(cfg)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want message containing %q", err, tc.want)
				}
			})
		})
	}
}

func TestConstructedService_InstallPackagedFactoryDefinitionsErrorsMatchPackageCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		install   factorydefinitionscli.InstallPackagedFactoryOperation
		wantIs    error
		wantWraps string
	}{
		{
			name: "named factory already exists",
			install: rejectingInstallOperation(factorydefinitions.ErrNamedFactoryAlreadyExists),
			wantIs: factorydefinitions.ErrNamedFactoryAlreadyExists,
		},
		{
			name: "factory distribute failed",
			install: rejectingInstallOperation(factorydefinitions.ErrFactoryDistributeFailed),
			wantIs: factorydefinitions.ErrFactoryDistributeFailed,
		},
		{
			name: "incompatible distribute options",
			install: rejectingInstallOperation(factorydefinitions.ErrIncompatibleFactoryDistributeOptions),
			wantIs: factorydefinitions.ErrIncompatibleFactoryDistributeOptions,
		},
		{
			name: "unknown packaged factory identity",
			install: rejectingInstallOperation(&factorydefinitions.UnknownPackagedFactoryError{
				Name:      "@you/missing",
				Available: []string{"@you/goal"},
			}),
			wantIs: factorydefinitions.ErrUnknownPackagedFactoryIdentity,
		},
		{
			name:      "wrapped install failure",
			install:   rejectingInstallOperation(errors.New("packaged factory install failed")),
			wantWraps: "install packaged factory: packaged factory install failed",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			service := constructedDefinitionsCLIService(t, tc.install)
			cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
				Context: ctx(t),
				HomeDir: "/home/operator",
				Package: "@you/goal",
			}
			assertInstallPackagedFactoryParity(t, service, tc.install, cfg, func() {
				var output bytes.Buffer
				cfg.Output = &output
				err := service.InstallPackagedFactory(cfg)
				if err == nil {
					t.Fatal("InstallPackagedFactory() error = nil, want failure")
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Fatalf("error = %v, want %v", err, tc.wantIs)
				}
				if tc.wantWraps != "" && err.Error() != tc.wantWraps {
					t.Fatalf("error = %q, want %q", err.Error(), tc.wantWraps)
				}
				if output.Len() != 0 {
					t.Fatalf("stdout = %q, want empty on failure", output.String())
				}
			})
		})
	}
}

func TestConstructedService_InstallPackagedFactoryVerboseDiagnosticsMatchPackageCommand(t *testing.T) {
	t.Parallel()

	install := successInstallOperation(factorydefinitions.PackagedFactoryInstallCreated)
	service := constructedDefinitionsCLIService(t, install)
	baseCfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx(t),
		HomeDir: "/home/operator",
		Package: "@you/goal",
		Verbose: true,
	}

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		var serviceDiag, commandDiag bytes.Buffer
		var serviceOut, commandOut bytes.Buffer
		serviceCfg := baseCfg
		serviceCfg.Output = &serviceOut
		serviceCfg.Diagnostics = &serviceDiag
		if err := service.InstallPackagedFactory(serviceCfg); err != nil {
			t.Fatalf("service.InstallPackagedFactory() error = %v", err)
		}

		commandCfg := baseCfg
		commandCfg.Output = &commandOut
		commandCfg.Diagnostics = &commandDiag
		if err := factorydefinitionscli.InstallPackagedFactory(commandCfg, install); err != nil {
			t.Fatalf("InstallPackagedFactory() error = %v", err)
		}
		if serviceOut.String() != commandOut.String() {
			t.Fatalf("service output = %q, command output = %q", serviceOut.String(), commandOut.String())
		}
		for _, want := range []string{
			"init packaged factory request name=@you/goal",
			"init packaged factory complete name=@you/goal",
			"outcome=created",
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

		installErr := errors.New("packaged factory install failed")
		failingInstall := rejectingInstallOperation(installErr)
		failingService := constructedDefinitionsCLIService(t, failingInstall)

		var serviceDiag, commandDiag bytes.Buffer
		serviceCfg := baseCfg
		serviceCfg.Output = io.Discard
		serviceCfg.Diagnostics = &serviceDiag
		serviceErr := failingService.InstallPackagedFactory(serviceCfg)

		commandCfg := baseCfg
		commandCfg.Output = io.Discard
		commandCfg.Diagnostics = &commandDiag
		commandErr := factorydefinitionscli.InstallPackagedFactory(commandCfg, failingInstall)

		if (serviceErr == nil) != (commandErr == nil) {
			t.Fatalf("service error = %v, command error = %v", serviceErr, commandErr)
		}
		if serviceErr != nil && serviceErr.Error() != commandErr.Error() {
			t.Fatalf("service error = %q, command error = %q", serviceErr.Error(), commandErr.Error())
		}
		for _, want := range []string{
			"init packaged factory request name=@you/goal",
			"init packaged factory failed name=@you/goal",
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

func TestConstructedService_InstallPackagedFactoryHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	install := func(
		callCtx context.Context,
		_ factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		if err := callCtx.Err(); err != nil {
			return factorydefinitions.InstallPackagedFactoryResult{}, err
		}
		return factorydefinitions.InstallPackagedFactoryResult{}, nil
	}
	service := constructedDefinitionsCLIService(t, install)
	cfg := factorydefinitionscli.InstallPackagedFactoryConfig{
		Context: ctx,
		HomeDir: "/home/operator",
		Package: "@you/goal",
	}

	var serviceOut bytes.Buffer
	serviceCfg := cfg
	serviceCfg.Output = &serviceOut
	serviceErr := service.InstallPackagedFactory(serviceCfg)
	if !errors.Is(serviceErr, context.Canceled) {
		t.Fatalf("service error = %v, want context.Canceled", serviceErr)
	}
	if serviceOut.Len() != 0 {
		t.Fatalf("service output = %q, want empty on cancellation", serviceOut.String())
	}

	var commandOut bytes.Buffer
	commandCfg := cfg
	commandCfg.Output = &commandOut
	commandErr := factorydefinitionscli.InstallPackagedFactory(commandCfg, install)
	if !errors.Is(commandErr, context.Canceled) {
		t.Fatalf("InstallPackagedFactory() error = %v, want context.Canceled", commandErr)
	}
	if commandOut.Len() != 0 {
		t.Fatalf("command output = %q, want empty on cancellation", commandOut.String())
	}
}

func successInstallOperation(
	outcome factorydefinitions.PackagedFactoryInstallOutcome,
	formats ...factorydefinitions.PackagedFactoryFormat,
) factorydefinitionscli.InstallPackagedFactoryOperation {
	format := factorydefinitions.PackagedFactoryFormatJSON
	if len(formats) > 0 {
		format = formats[0]
	}
	return func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{
			Definition: factorydefinitions.DistributedFactoryDefinitionFacts{
				Name:       "@you/goal",
				FactoryDir: "/home/operator/.you-agent-factory/factories/@you/goal",
			},
			Outcome: outcome,
			Format:  format,
		}, nil
	}
}

func rejectingInstallOperation(
	installErr error,
) factorydefinitionscli.InstallPackagedFactoryOperation {
	return func(
		context.Context,
		factorydefinitions.InstallPackagedFactoryRequest,
	) (factorydefinitions.InstallPackagedFactoryResult, error) {
		return factorydefinitions.InstallPackagedFactoryResult{}, installErr
	}
}
