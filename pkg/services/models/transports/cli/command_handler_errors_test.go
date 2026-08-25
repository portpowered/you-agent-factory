package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func TestModelsTransportErrorSummaryIdentifiesTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	if got := modelsTransportErrorSummary(context.DeadlineExceeded); got != "error=timeout" {
		t.Fatalf("deadline summary = %q, want error=timeout", got)
	}
	if got := modelsTransportErrorSummary(context.Canceled); got != "error=canceled" {
		t.Fatalf("cancellation summary = %q, want error=canceled", got)
	}
	if got := modelsTransportErrorSummary(errors.New("connection refused")); got != "error=unreachable" {
		t.Fatalf("transport summary = %q, want error=unreachable", got)
	}
}

func TestModelsPullDiagnosticsIdentifyOrdinaryClientTimeout(t *testing.T) {
	t.Parallel()
	protocol, err := clihttp.NewProtocol(&http.Client{
		Timeout: 5 * time.Millisecond,
		Transport: modelsPullRoundTripper(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}),
	}, testHTTPClock{})
	if err != nil {
		t.Fatalf("timeout protocol: %v", err)
	}
	var diagnostics bytes.Buffer
	_, err = pullModel(pullOptions{
		Context: context.Background(), Server: "http://factory.test",
		ModelName: "OMNIVOICE_Q4_K_M", Diagnostics: &diagnostics, Verbose: true,
		HTTP: protocol,
	})
	if err == nil {
		t.Fatal("pull error = nil, want ordinary client timeout")
	}
	if !strings.Contains(diagnostics.String(), "error=timeout") {
		t.Fatalf("timeout diagnostics = %q, want error=timeout", diagnostics.String())
	}
}

func TestPull_JSONWritesPullMetadataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/OMNIVOICE_Q4_K_M/pull" {
			t.Fatalf("path = %q, want pull path", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/OMNIVOICE_Q4_K_M/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}]}`)
	}))
	defer server.Close()

	serverBase := strings.TrimSuffix(server.URL, "/")
	var out bytes.Buffer
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(),
		ModelName: "OMNIVOICE_Q4_K_M",
		Server:    serverBase,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, want := range []string{"OMNIVOICE_Q4_K_M", `"outcome":"PULLED"`} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestModelsList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q, want /models", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"results":[{"name":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[]}]}`)
	}))
	defer server.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background(),
		Server:      strings.TrimSuffix(server.URL, "/"),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	var response factoryapi.ListModelsResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	assertDiagnosticsContains(t, diagnostics.String(), []string{
		"models list request",
		"endpointPath=/models",
		"server=",
		"models list response",
		"status=200",
		"resultCount=1",
	})
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
}

func TestModelsVerboseLogsInspectInvokeAndPullMetadataWithoutInputText(t *testing.T) {
	streamFile := filepath.Join(t.TempDir(), "generated.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF-test-audio"), 0o600); err != nil {
		t.Fatalf("write test audio stream: %v", err)
	}
	var inspectRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models/OMNIVOICE_Q4_K_M":
			inspectRequests.Add(1)
			_, _ = io.WriteString(w, `{"name":"OMNIVOICE_Q4_K_M","managedRuntime":{"identity":"OMNIVOICE_Q4_K_M","readinessState":"READY","lifecycleState":"NOT_INSTALLED","locality":"LOCAL","supportedOperations":[{"name":"TTS"}],"diagnostics":{}},"providerLocality":"LOCAL","status":"READY","loadState":"UNLOADED","operations":[{"name":"TTS"}],"modalities":["TEXT"],"resources":[],"capabilities":[],"diagnostics":{}}`)
		case "/models/OMNIVOICE_Q4_K_M/pull":
			_, _ = io.WriteString(w, `{"modelName":"OMNIVOICE_Q4_K_M","providerLocality":"LOCAL","outcome":"PULLED","cachePath":"/tmp/models/ghp_successResponseToken1234567890/rev1","revision":"rev1","downloadedFiles":[{"path":"omnivoice-base-Q4_K_M.gguf","bytes":407}],"managedRuntimePull":{"identity":"OMNIVOICE_Q4_K_M","pullOutcome":"INSTALLED_SUCCESSFULLY","readinessState":"READY"}}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
		return modelinference.Result{
			ModelName:  modelName,
			Worker:     "tts-worker",
			Operation:  request.Operation,
			StreamFile: streamFile,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeAudio,
				File: "artifacts/sensitive-generated-output.wav",
			}},
		}, nil
	}))

	serverBase := strings.TrimSuffix(server.URL, "/")
	var diagnostics bytes.Buffer
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:   "OMNIVOICE_Q4_K_M",
		Operation:   "TTS",
		Text:        "secret direct input",
		FactoryDir:  t.TempDir(),
		Logger:      zap.NewNop(),
		OutputPath:  filepath.Join(t.TempDir(), "speech.wav"),
		Output:      io.Discard,
		Verbose:     true,
		Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background(), ModelName: "OMNIVOICE_Q4_K_M", Server: serverBase, Output: io.Discard, Verbose: true, Diagnostics: &diagnostics}); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect request",
		"modelName=\"OMNIVOICE_Q4_K_M\"",
		"readiness=READY",
		"models invoke bootstrap request",
		"operation=\"TTS\"",
		"models invoke bootstrap response",
		"models pull request",
		"pullOutcome=INSTALLED_SUCCESSFULLY",
		"readiness=READY",
		"downloadedFiles=1",
	})
	for _, forbidden := range []string{"secret direct input", "sensitive-generated-output.wav", "ghp_successResponseToken1234567890"} {
		if strings.Contains(diag, forbidden) {
			t.Fatalf("diagnostics leaked model input, response content, or token %q:\n%s", forbidden, diag)
		}
	}
	if got := inspectRequests.Load(); got != 1 {
		t.Fatalf("inspect requests = %d, want 1", got)
	}
}

func TestModelsFailureOmitsNonJSONResponseBody(t *testing.T) {
	responseBody := "opaque-secret-response-marker"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, responseBody)
	}))
	defer server.Close()

	var diagnostics bytes.Buffer
	_, err := queryModel(queryOptions{
		Context:     context.Background(),
		HTTP:        testHTTPProtocol(t),
		Server:      strings.TrimSuffix(server.URL, "/"),
		ModelName:   "broken",
		Verbose:     true,
		Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected queryModel to fail")
	}
	gotErr := err.Error()
	if !strings.Contains(gotErr, "models request failed (502): response body was not a structured API error") {
		t.Fatalf("error = %q, want safe non-JSON response summary", gotErr)
	}
	if strings.Contains(gotErr, responseBody) {
		t.Fatalf("error included raw response body")
	}
	diag := diagnostics.String()
	assertDiagnosticsContains(t, diag, []string{
		"models inspect response",
		"endpointPath=/models/broken",
		"status=502",
		"responseBytes=29",
	})
	if strings.Contains(diag, responseBody) {
		t.Fatalf("diagnostics leaked model input or response content:\n%s", diag)
	}
}

func TestManagedRuntimePullResponseErrorPreservesOutcomeDetails(t *testing.T) {
	err := managedRuntimePullResponseError(http.StatusUnprocessableEntity, []byte(`{
		"managedRuntimePull": {
			"identity": "OMNIVOICE_Q4_K_M",
			"pullOutcome": "SOURCE_FETCH_FAILED",
			"readinessState": "FAILED",
			"pullDiagnostics": {
				"modelName": "OMNIVOICE_Q4_K_M",
				"resolvedRepository": "owner/repo",
				"revision": "rev-1",
				"file": "weights.gguf",
				"operation": "download asset",
				"requestUrl": "https://assets.example.test/owner/repo/weights.gguf?download=true",
				"upstreamStatusCode": 502
			}
		}
	}`))
	if err == nil {
		t.Fatal("managedRuntimePullResponseError() = nil, want classified failure")
	}
	if got, want := err.Error(), "managed runtime pull failed (pullOutcome=SOURCE_FETCH_FAILED readinessState=FAILED)"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("error = %T, want coded model pull failure", err)
	}
	if coded.CLIErrorCode() != managedRuntimePullFailureCode ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest ||
		coded.CLIErrorMessage() != err.Error() {
		t.Fatalf("coded failure = (%q, %q, %q), want safe outcome diagnostic", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage())
	}
	var diagnostics *modelinference.PullDiagnosticsError
	if !errors.As(err, &diagnostics) || diagnostics == nil {
		t.Fatalf("error = %T, want structured pull diagnostics cause", err)
	}
	if !strings.Contains(diagnostics.Error(), "repository=owner/repo") ||
		!strings.Contains(diagnostics.Error(), "status=502") ||
		!strings.Contains(diagnostics.Error(), "operation=download asset") {
		t.Fatalf("diagnostics = %q, want repository, operation, and status", diagnostics)
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

func TestMapModelsRootErrorClassifiesMissingCacheWithModelName(t *testing.T) {
	t.Parallel()

	cause := fmt.Errorf("remove model: %w: OMNIVOICE_Q4_K_M", modelinference.ErrModelCacheNotFound)
	mapped := mapModelsRootError(cause)
	if mapped == nil || !errors.Is(mapped, modelinference.ErrModelCacheNotFound) {
		t.Fatalf("mapped cache error = %v, want preserved cache sentinel", mapped)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(mapped, &coded) {
		t.Fatalf("mapped cache error = %T, want CLI diagnostic", mapped)
	}
	if coded.CLIErrorCode() != modelsRootCacheNotFoundCode ||
		coded.CLIErrorFamily() != factoryapi.ErrorFamilyNotFound ||
		coded.CLIErrorMessage() != "model cache is not installed; run you models pull OMNIVOICE_Q4_K_M first" {
		t.Fatalf("cache diagnostic = (%q, %q, %q), want model-specific not-found guidance", coded.CLIErrorCode(), coded.CLIErrorFamily(), coded.CLIErrorMessage())
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

func TestModelsCLISlotMappingPreservesOptionalMetadataShape(t *testing.T) {
	t.Parallel()

	converted := slotsToGenerated([]modelinference.OperationSlot{
		{Name: "opaque"},
		{Name: "text", Modality: modelinference.ModalityText, Repeatable: true, MediaTypes: []string{"text/plain"}},
	})
	if len(converted) != 2 || converted[0].Modality != nil || converted[0].Repeatable != nil || converted[0].MediaTypes != nil {
		t.Fatalf("optional slot mapping = %#v, want nil optional metadata", converted)
	}
	if converted[1].Modality == nil || *converted[1].Modality != factoryapi.ModelInvocationContentType(modelinference.ModalityText) || converted[1].Repeatable == nil || !*converted[1].Repeatable || converted[1].MediaTypes == nil {
		t.Fatalf("declared slot mapping = %#v, want modality/repeatable/media metadata", converted[1])
	}
}

func TestReadModelsInvokeInputsClearsManifestOperationForNamedInput(t *testing.T) {
	t.Parallel()

	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}},
			{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
			{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeInputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		},
		[]resolvedinput.Candidate{
			{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("llm")},
			{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue(modelinference.OperationTTS)},
			{InputID: modelsInvokeTextID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("")},
			{InputID: modelsInvokeInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue([]string{"prompt=Write a haiku"})},
			{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringArrayValue(nil)},
		},
	)
	if err != nil {
		t.Fatalf("resolve invoke inputs: %v", err)
	}

	got, err := readModelsInvokeInputs(inputs)
	if err != nil {
		t.Fatalf("readModelsInvokeInputs: %v", err)
	}
	if got.operation != "" {
		t.Fatalf("operation = %q, want empty so the built-in alias can infer OMNI", got.operation)
	}
	if !reflect.DeepEqual(got.inputMappings, []string{"prompt=Write a haiku"}) {
		t.Fatalf("input mappings = %#v, want named prompt", got.inputMappings)
	}
}

func TestInferGenericCLIModelOperationUsesBuiltInAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{name: "llm", model: "  LLM ", want: modelinference.OperationOMNI},
		{name: "asr", model: modelinference.BuiltInModelNameASR, want: modelinference.OperationASR},
		{name: "tts", model: modelinference.BuiltInModelNameTTS, want: modelinference.OperationTTS},
		{name: "embed", model: modelinference.BuiltInModelNameEmbed, want: modelinference.OperationEMBED},
		{name: "authored", model: "custom-model", want: ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := inferGenericCLIModelOperation(test.model); got != test.want {
				t.Fatalf("inferGenericCLIModelOperation(%q) = %q, want %q", test.model, got, test.want)
			}
		})
	}
}

func TestReadModelsInvokeOutputsSupportsScalarAndNamedForms(t *testing.T) {
	t.Parallel()

	resolve := func(t *testing.T, outputKind resolvedinput.ValueKind, output resolvedinput.Value, legacy []string) resolvedinput.Inputs {
		t.Helper()
		definitions := []resolvedinput.Definition{{ID: modelsInvokeOutputID, Kind: outputKind, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}}}
		candidates := []resolvedinput.Candidate{{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceCLIFlag, Value: output}}
		if legacy != nil {
			definitions = append(definitions, resolvedinput.Definition{ID: modelsInvokeOutputMapID, Kind: resolvedinput.ValueKindStringArray, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}})
			candidates = append(candidates, resolvedinput.Candidate{InputID: modelsInvokeOutputMapID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringArrayValue(legacy)})
		}
		inputs, err := resolvedinput.Resolve(definitions, candidates)
		if err != nil {
			t.Fatalf("resolve output inputs: %v", err)
		}
		return inputs
	}

	t.Run("scalar path remains supported", func(t *testing.T) {
		path, mappings, err := readModelsInvokeOutputs(resolve(t, resolvedinput.ValueKindString, resolvedinput.StringValue("answer.txt"), nil))
		if err != nil || path != "answer.txt" || len(mappings) != 0 {
			t.Fatalf("scalar output = path:%q mappings:%#v error:%v, want path", path, mappings, err)
		}
	})
	t.Run("named repeatable values", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"text=answer.txt"}), []string{"usage=usage.json"})
		path, mappings, err := readModelsInvokeOutputs(inputs)
		if err != nil || path != "" || !reflect.DeepEqual(mappings, []string{"text=answer.txt", "usage=usage.json"}) {
			t.Fatalf("named outputs = path:%q mappings:%#v error:%v", path, mappings, err)
		}
	})
	t.Run("path and mapping conflict", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"answer.txt", "text=answer.txt"}), nil)
		if _, _, err := readModelsInvokeOutputs(inputs); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("path/mapping conflict = %v, want conflict", err)
		}
	})
	t.Run("second unqualified path is rejected", func(t *testing.T) {
		inputs := resolve(t, resolvedinput.ValueKindStringArray, resolvedinput.StringArrayValue([]string{"one.txt", "two.txt"}), nil)
		if _, _, err := readModelsInvokeOutputs(inputs); err == nil || !strings.Contains(err.Error(), "after the first unqualified path") {
			t.Fatalf("multiple paths = %v, want repeatable path error", err)
		}
	})
}

func TestCommandHandlerTransformsRemoveArguments(t *testing.T) {
	server := "http://127.0.0.1:7437"
	called := false
	handler := NewCommandHandler(
		commandServiceFake{remove: func(cfg RemoveConfig) error {
			called = true
			if cfg.ModelName != "model-c" || cfg.Server != server || cfg.Context.Err() != context.Canceled {
				t.Fatalf("RemoveConfig = %#v", cfg)
			}
			return nil
		}},
		func(*cobra.Command) io.Writer { return io.Discard },
		nil,
		nil,
		nil,
	)
	cmd := &cobra.Command{Use: "models"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd.SetContext(ctx)
	cmd.SetOut(io.Discard)
	_, _, inherited := resolvedModelsHandlerInputs(t, server)
	removeInputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{ID: modelsRemoveNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourcePositionalArgument}}},
		[]resolvedinput.Candidate{{InputID: modelsRemoveNameInputID, Source: resolvedinput.SourcePositionalArgument, Value: resolvedinput.StringValue("model-c")}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.Remove(cmd, removeInputs, inherited); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("remove service operation was not called")
	}
}
