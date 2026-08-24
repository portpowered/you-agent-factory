package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func assertTransformedInvokeConfig(
	t *testing.T,
	server string,
	logger *zap.Logger,
	diagnostics *bytes.Buffer,
) func(InvokeConfig) error {
	t.Helper()
	return func(cfg InvokeConfig) error {
		if cfg.ModelName != "OMNIVOICE_Q4_K_M" || cfg.Operation != "TTS" || cfg.Text != "hello" || cfg.OutputPath != "speech.wav" {
			t.Fatalf("InvokeConfig command values = %#v", cfg)
		}
		if !reflect.DeepEqual(cfg.InputMappings, []string{"audio=@meeting.wav", "prompt=hint"}) {
			t.Fatalf("InvokeConfig input mappings = %#v", cfg.InputMappings)
		}
		if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
			t.Fatalf("InvokeConfig global values = %#v", cfg)
		}
		if cfg.FactoryDir != "/factory" || cfg.HomeDir != "/home/tester" || cfg.Logger != logger || cfg.Diagnostics != diagnostics {
			t.Fatalf("InvokeConfig dependencies = %#v", cfg)
		}
		if cfg.ModelCacheDir != "/managed/models" {
			t.Fatalf("InvokeConfig model cache dir = %q, want /managed/models", cfg.ModelCacheDir)
		}
		return nil
	}
}

func TestCommandHandlerRequiresInjectedModelsService(t *testing.T) {
	handler := NewCommandHandler(nil, nil, nil, nil, nil)
	cmd := &cobra.Command{Use: "models"}
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "list",
			run:  func() error { return handler.List(cmd, resolvedinput.Inputs{}, resolvedinput.Inputs{}) },
			want: "models list service is required",
		},
		{
			name: "inspect",
			run:  func() error { return handler.Inspect(cmd, resolvedinput.Inputs{}, resolvedinput.Inputs{}) },
			want: "models inspect service is required",
		},
		{
			name: "invoke",
			run:  func() error { return handler.Invoke(cmd, resolvedinput.Inputs{}, resolvedinput.Inputs{}) },
			want: "models invoke service is required",
		},
		{
			name: "pull",
			run:  func() error { return handler.Pull(cmd, resolvedinput.Inputs{}, resolvedinput.Inputs{}) },
			want: "models pull service is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCommandHandlerReportsMissingLocalInputs(t *testing.T) {
	handler := newRejectingModelsCommandHandler(t)
	cmd := modelsCommandWithFactoryContext()
	_, _, inherited := resolvedModelsHandlerInputs(t, "http://127.0.0.1:7437")
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "inspect missing model name",
			run: func() error {
				return handler.Inspect(cmd, resolvedinput.Inputs{}, inherited)
			},
			want: "read models inspect model name",
		},
		{
			name: "pull missing model name",
			run: func() error {
				return handler.Pull(cmd, resolvedinput.Inputs{}, inherited)
			},
			want: "read models pull model name",
		},
		{
			name: "invoke missing model name",
			run: func() error {
				return handler.Invoke(cmd, resolvedinput.Inputs{}, inherited)
			},
			want: "read models invoke model name",
		},
		{
			name: "invoke missing operation",
			run: func() error {
				return handler.Invoke(cmd, resolvedInvokePartialInputs(t, true, false, true, true), inherited)
			},
			want: "read models invoke operation",
		},
		{
			name: "invoke missing text",
			run: func() error {
				return handler.Invoke(cmd, resolvedInvokePartialInputs(t, true, true, false, true), inherited)
			},
			want: "read models invoke text",
		},
		{
			name: "invoke missing output",
			run: func() error {
				return handler.Invoke(cmd, resolvedInvokePartialInputs(t, true, true, true, false), inherited)
			},
			want: "read models invoke output",
		},
	}
	assertCommandHandlerErrors(t, cases)
}

func TestCommandHandlerReportsMissingInheritedInputs(t *testing.T) {
	handler := newRejectingModelsCommandHandler(t)
	cmd := modelsCommandWithFactoryContext()
	inspectInputs, pullInputs, _ := resolvedModelsHandlerInputs(t, "http://127.0.0.1:7437")
	invokeInputs, _ := resolvedInvokeHandlerInputs(t, "http://127.0.0.1:7437")
	cases := []struct {
		name string
		run  func() error
		want string
	}{
		{
			name: "list missing inherited server",
			run: func() error {
				return handler.List(cmd, resolvedinput.Inputs{}, resolvedinput.Inputs{})
			},
			want: "resolve models list inputs",
		},
		{
			name: "inspect missing inherited common inputs",
			run: func() error {
				return handler.Inspect(cmd, inspectInputs, resolvedinput.Inputs{})
			},
			want: "resolve models inspect inputs",
		},
		{
			name: "pull missing inherited common inputs",
			run: func() error {
				return handler.Pull(cmd, pullInputs, resolvedinput.Inputs{})
			},
			want: "resolve models pull inputs",
		},
		{
			name: "invoke missing inherited common inputs",
			run: func() error {
				return handler.Invoke(cmd, invokeInputs, resolvedinput.Inputs{})
			},
			want: "resolve models invoke inputs",
		},
	}
	assertCommandHandlerErrors(t, cases)
}

func newRejectingModelsCommandHandler(t *testing.T) *CommandHandler {
	t.Helper()
	service := commandServiceFake{
		list:    func(ListConfig) error { t.Fatal("list must not run"); return nil },
		inspect: func(InspectConfig) error { t.Fatal("inspect must not run"); return nil },
		invoke:  func(InvokeConfig) error { t.Fatal("invoke must not run"); return nil },
		pull:    func(PullConfig) error { t.Fatal("pull must not run"); return nil },
	}
	return NewCommandHandler(
		service,
		func(*cobra.Command) io.Writer { return io.Discard },
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
	)
}

func modelsCommandWithFactoryContext() *cobra.Command {
	cmd := &cobra.Command{Use: "models"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	return cmd
}

func assertCommandHandlerErrors(t *testing.T, cases []struct {
	name string
	run  func() error
	want string
}) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func resolvedInvokePartialInputs(
	t *testing.T,
	includeName bool,
	includeOperation bool,
	includeText bool,
	includeOutput bool,
) resolvedinput.Inputs {
	t.Helper()
	definitions := make([]resolvedinput.Definition, 0, 4)
	candidates := make([]resolvedinput.Candidate, 0, 4)
	add := func(id string, source resolvedinput.Source, value string) {
		definitions = append(definitions, resolvedinput.Definition{
			ID: id, Kind: resolvedinput.ValueKindString,
			Precedence: []resolvedinput.Source{source},
		})
		candidates = append(candidates, resolvedinput.Candidate{
			InputID: id, Source: source,
			Value: resolvedinput.StringValue(value),
		})
	}
	if includeName {
		add(modelsInvokeNameInputID, resolvedinput.SourcePositionalArgument, "OMNIVOICE_Q4_K_M")
	}
	if includeOperation {
		add(modelsInvokeOperationID, resolvedinput.SourceCLIFlag, "TTS")
	}
	if includeText {
		add(modelsInvokeTextID, resolvedinput.SourceCLIFlag, "hello")
	}
	if includeOutput {
		add(modelsInvokeOutputID, resolvedinput.SourceCLIFlag, "speech.wav")
	}
	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("resolve partial invoke inputs: %v", err)
	}
	return inputs
}

func TestCommandHandlerInvokeRequiresInjectedDependencies(t *testing.T) {
	service := commandServiceFake{
		invoke: func(InvokeConfig) error {
			t.Fatal("invoke must not run without injected dependencies")
			return nil
		},
	}
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	invokeInputs, inherited := resolvedInvokeHandlerInputs(t, "http://127.0.0.1:7437")
	cases := []struct {
		name    string
		handler *CommandHandler
		want    string
	}{
		{
			name: "logger builder",
			handler: NewCommandHandler(service, nil, func() (string, error) { return "/home", nil },
				func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
					return operatorconfig.ResolvedDefaults{}, nil
				}, nil),
			want: "model invocation logger builder is required",
		},
		{
			name: "home directory",
			handler: NewCommandHandler(service, nil, nil,
				func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
					return operatorconfig.ResolvedDefaults{}, nil
				},
				func() (*zap.Logger, error) { return zap.NewNop(), nil }),
			want: "model invocation home directory resolver is required",
		},
		{
			name: "operator defaults",
			handler: NewCommandHandler(service, nil, func() (string, error) { return "/home", nil }, nil,
				func() (*zap.Logger, error) { return zap.NewNop(), nil }),
			want: "model invocation operator defaults resolver is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.handler.Invoke(cmd, invokeInputs, inherited)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCommandHandlerInvokePreservesDependencyFailures(t *testing.T) {
	service := commandServiceFake{
		invoke: func(InvokeConfig) error {
			t.Fatal("invoke must not run after dependency failure")
			return nil
		},
	}
	cmd := &cobra.Command{Use: "invoke"}
	cmd.SetContext(startupcli.WithWorkingDirectory(context.Background(), "/factory"))
	invokeInputs, inherited := resolvedInvokeHandlerInputs(t, "http://127.0.0.1:7437")

	loggerErr := errors.New("logger unavailable")
	handler := NewCommandHandler(
		service,
		nil,
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return nil, loggerErr },
	)
	if err := handler.Invoke(cmd, invokeInputs, inherited); !errors.Is(err, loggerErr) {
		t.Fatalf("logger failure = %v, want %v", err, loggerErr)
	}

	homeErr := errors.New("home unavailable")
	handler = NewCommandHandler(
		service,
		nil,
		func() (string, error) { return "", homeErr },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, nil
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
	)
	err := handler.Invoke(cmd, invokeInputs, inherited)
	if !errors.Is(err, homeErr) || !strings.Contains(err.Error(), "resolve process home directory") {
		t.Fatalf("home failure = %v, want wrapped home error", err)
	}

	defaultsErr := errors.New("defaults unavailable")
	handler = NewCommandHandler(
		service,
		nil,
		func() (string, error) { return "/home/tester", nil },
		func(*cobra.Command, string) (operatorconfig.ResolvedDefaults, error) {
			return operatorconfig.ResolvedDefaults{}, defaultsErr
		},
		func() (*zap.Logger, error) { return zap.NewNop(), nil },
	)
	if err := handler.Invoke(cmd, invokeInputs, inherited); !errors.Is(err, defaultsErr) {
		t.Fatalf("defaults failure = %v, want %v", err, defaultsErr)
	}
}

func TestCommandHandlerOmitsManifestDefaultServerFromModelsConfig(t *testing.T) {
	t.Parallel()

	var gotServer string
	handler := NewCommandHandler(
		commandServiceFake{
			list: func(cfg ListConfig) error {
				gotServer = cfg.Server
				return nil
			},
		},
		nil, nil, nil, nil,
	)
	inherited, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{
				ID: serverInputID, Kind: resolvedinput.ValueKindString,
				Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
			},
			{
				ID: jsonInputID, Kind: resolvedinput.ValueKindBool,
				Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
			},
			{
				ID: verboseInputID, Kind: resolvedinput.ValueKindBool,
				Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
			},
			{
				ID: debugInputID, Kind: resolvedinput.ValueKindBool,
				Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault},
			},
		},
		[]resolvedinput.Candidate{
			{
				InputID: serverInputID, Source: resolvedinput.SourceManifestDefault,
				Value: resolvedinput.StringValue("http://localhost:7437"),
			},
			{
				InputID: jsonInputID, Source: resolvedinput.SourceManifestDefault,
				Value: resolvedinput.BoolValue(false),
			},
			{
				InputID: verboseInputID, Source: resolvedinput.SourceManifestDefault,
				Value: resolvedinput.BoolValue(false),
			},
			{
				InputID: debugInputID, Source: resolvedinput.SourceManifestDefault,
				Value: resolvedinput.BoolValue(false),
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "models"}
	cmd.SetOut(io.Discard)
	if err := handler.List(cmd, resolvedinput.Inputs{}, inherited); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotServer != "" {
		t.Fatalf("server = %q, want empty for manifest-default --server", gotServer)
	}
}

func TestModelsClientErrorPreservesTypedInvocationDiagnostics(t *testing.T) {
	t.Parallel()

	cause := errors.New("validation cause")
	message := "required input slot is missing: audio"
	for _, test := range []struct {
		name       string
		class      modelinference.InvocationFailureClass
		code       string
		family     factoryapi.ErrorFamily
		badRequest bool
	}{
		{
			name:       "missing required slot",
			class:      modelinference.InvocationFailureClassInvalidSlot,
			code:       "BAD_REQUEST",
			family:     factoryapi.ErrorFamilyBadRequest,
			badRequest: true,
		},
		{
			name:   "unknown model reference",
			class:  modelinference.InvocationFailureClassInvalidModelReference,
			code:   "MODEL_NOT_AVAILABLE",
			family: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:   "backend readiness",
			class:  modelinference.InvocationFailureClassBackendReadiness,
			code:   "MODEL_BACKEND_NOT_READY",
			family: factoryapi.ErrorFamilyInternalServerError,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			failure := &modelinference.InvocationFailure{
				Class: test.class, Message: message, Cause: cause,
			}
			wrapped := fmt.Errorf("models invocation: %w", failure)
			mapped := mapModelsClientError(wrapped)

			var classified *modelinference.InvocationFailure
			if !errors.As(mapped, &classified) || classified != failure {
				t.Fatalf("mapped error = %v, failure = %#v, want the original typed failure", mapped, classified)
			}
			if !errors.Is(mapped, cause) {
				t.Fatalf("mapped error = %v, want the original cause in the chain", mapped)
			}
			var coded interface {
				CLIErrorCode() string
				CLIErrorFamily() factoryapi.ErrorFamily
				CLIErrorMessage() string
			}
			if !errors.As(mapped, &coded) {
				t.Fatalf("mapped error = %T, want CLI diagnostic contract", mapped)
			}
			if coded.CLIErrorCode() != test.code || coded.CLIErrorFamily() != test.family || coded.CLIErrorMessage() != message {
				t.Fatalf("CLI fields = (%q, %q, %q), want (%q, %q, %q)", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage(), test.code, test.family, message)
			}
			if test.badRequest && coded.CLIErrorFamily() == factoryapi.ErrorFamilyInternalServerError {
				t.Fatal("client validation was classified as INTERNAL_SERVER_ERROR")
			}
		})
	}
}

func TestModelsRootErrorDoesNotReclassifyInvocationFailuresOutsideClientPaths(t *testing.T) {
	t.Parallel()

	failure := &modelinference.InvocationFailure{
		Class:   modelinference.InvocationFailureClassInvalidSlot,
		Message: "required input slot is missing: audio",
	}
	if mapped := mapModelsRootError(failure); mapped != failure {
		t.Fatalf("mapModelsRootError() = %T, want the original failure for non-invocation paths", mapped)
	}
	if mapped := mapModelsRootError(modelinference.ErrNotFound); mapped == nil || !errors.Is(mapped, modelinference.ErrNotFound) {
		t.Fatalf("mapModelsRootError(ErrNotFound) = %v, want unchanged not-found identity", mapped)
	}
}
