package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
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

func TestMapModelsRootError_ClassifiesMissingFactoryLayoutWithSearchedRoot(t *testing.T) {
	t.Parallel()

	searchedRoot := `C:\workspace\project\factory`
	cause := fmt.Errorf("resolve current factory in %s: %w", searchedRoot, factorydefinitions.ErrFactoryLayoutNotFound)
	mapped := mapModelsRootError(cause)
	if mapped == nil {
		t.Fatal("mapModelsRootError() = nil, want classified failure")
	}
	if !errors.Is(mapped, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("mapped error = %v, want ErrFactoryLayoutNotFound cause", mapped)
	}

	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(mapped, &coded) {
		t.Fatalf("mapped error = %T, want CLI-coded failure", mapped)
	}
	if coded.CLIErrorCode() != modelsFactoryLayoutNotFoundCode ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(coded.CLIErrorMessage(), searchedRoot) {
		t.Fatalf("coded failure = (%q, %q, %q), want not-found with searched root %q", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage(), searchedRoot)
	}

	var output bytes.Buffer
	if !clidiag.WriteFailure(&output, mapped) {
		t.Fatal("WriteFailure() = false, want one customer-visible diagnostic")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("diagnostic JSON = %q: %v", output.String(), err)
	}
	if response.Code != factoryapi.ErrorResponseCode(modelsFactoryLayoutNotFoundCode) ||
		response.Family != factoryapi.ErrorFamilyNotFound ||
		!strings.Contains(response.Message, searchedRoot) {
		t.Fatalf("customer response = %#v, want not-found classification and searched root %q", response, searchedRoot)
	}
}

func TestModelsFactoryLayoutNotFoundErrorHandlesNilAndPreservesCause(t *testing.T) {
	t.Parallel()

	var nilError *modelsFactoryLayoutNotFoundError
	if got := nilError.Error(); got != "Factory layout was not found" {
		t.Fatalf("nil layout error = %q, want stable fallback text", got)
	}
	if nilError.Unwrap() != nil || nilError.CLIErrorMessage() != "Factory layout was not found" {
		t.Fatalf("nil layout error methods = unwrap %v message %q", nilError.Unwrap(), nilError.CLIErrorMessage())
	}

	cause := errors.New("resolve current factory in C:\\workspace\\factory")
	mapped := &modelsFactoryLayoutNotFoundError{cause: cause}
	if mapped.Error() != cause.Error() || !errors.Is(mapped, cause) || mapped.CLIErrorCode() != modelsFactoryLayoutNotFoundCode || mapped.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("mapped layout error = %v, want cause and not-found diagnostic", mapped)
	}
}

func TestModelsInvocationDiagnosticCoversFailureClasses(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		class  modelinference.InvocationFailureClass
		code   string
		family factoryapi.ErrorFamily
	}{
		{name: "revision", class: modelinference.InvocationFailureClassRevisionResolution, code: "MODEL_NOT_AVAILABLE", family: factoryapi.ErrorFamilyNotFound},
		{name: "parameter", class: modelinference.InvocationFailureClassInvalidParameter, code: "BAD_REQUEST", family: factoryapi.ErrorFamilyBadRequest},
		{name: "offline cache", class: modelinference.InvocationFailureClassOfflineCache, code: "MODEL_OFFLINE_CACHE_UNAVAILABLE", family: factoryapi.ErrorFamilyConflict},
		{name: "backend protocol", class: modelinference.InvocationFailureClassBackendProtocol, code: "MODEL_BACKEND_FAILURE", family: factoryapi.ErrorFamilyInternalServerError},
		{name: "timeout", class: modelinference.InvocationFailureClassTimeout, code: "MODEL_INFERENCE_TIMEOUT", family: factoryapi.ErrorFamilyInternalServerError},
		{name: "configuration", class: modelinference.InvocationFailureClassConfiguration, code: "MODEL_CONFIGURATION_FAILURE", family: factoryapi.ErrorFamilyInternalServerError},
		{name: "unknown", class: modelinference.InvocationFailureClass("UNKNOWN"), code: "MODEL_INFERENCE_RUNTIME_FAILURE", family: factoryapi.ErrorFamilyInternalServerError},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			code, family := modelsInvocationDiagnostic(test.class)
			if code != test.code || family != test.family {
				t.Fatalf("modelsInvocationDiagnostic(%q) = (%q, %q), want (%q, %q)", test.class, code, family, test.code, test.family)
			}
		})
	}
}
