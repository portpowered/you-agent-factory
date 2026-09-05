// Package cli defines the Models service's CLI adapter.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	pullsupport "github.com/portpowered/infinite-you/pkg/services/models/internal/pullsupport"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

const modelsErrorBodyPreviewSize = 200

var (
	ErrModelNotFound      = errors.New("model not found")
	ErrModelCacheNotFound = errors.New("model cache not found")
	ErrModelCacheInUse    = errors.New("model cache is in use")
	ErrModelCacheUnsafe   = errors.New("model cache path is unsafe")
)

const managedRuntimePullFailureCode = "CLI_MODEL_PULL_FAILED"

type ListConfig struct {
	Context     context.Context
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type InspectConfig struct {
	Context     context.Context
	ModelName   string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type InvokeConfig struct {
	Context       context.Context
	ModelName     string
	Operation     string
	Text          string
	InputMappings []string
	// InputSpecs is the structured-input alias retained for direct callers of
	// the Models root. The Cobra adapter carries repeatable --input values in
	// InputMappings so legacy slot=value and JSON forms share one flag.
	InputSpecs []string
	// ParameterSpecs contains repeatable JSON-encoded generic operation
	// parameters. Each value preserves one parameter's name and JSON value.
	ParameterSpecs   []string
	OutputPath       string
	OutputMappings   []string
	Server           string
	FactoryDir       string
	WorkingDirectory string
	HomeDir          string
	OperatorDefaults operatorconfig.ResolvedDefaults
	Logger           *zap.Logger
	JSON             bool
	Verbose          bool
	Debug            bool
	Output           io.Writer
	Diagnostics      io.Writer
}

const modelInvocationValidationOnlyMode = "VALIDATION_ONLY"

// modelInvocationValidationResponse is deliberately not the inference result
// envelope. Metadata mode validates the request and reports that no inference
// was attempted, so callers cannot mistake a successful preflight for output.
type modelInvocationValidationResponse struct {
	ModelName         string `json:"modelName"`
	Operation         string `json:"operation"`
	Mode              string `json:"mode"`
	ValidationOnly    bool   `json:"validationOnly"`
	InferenceExecuted bool   `json:"inferenceExecuted"`
}

func validationOnlyModelInvoke(cfg InvokeConfig) bool {
	return cfg.JSON && strings.TrimSpace(cfg.OutputPath) == "" &&
		len(cfg.InputMappings) == 0 && len(cfg.InputSpecs) == 0 &&
		len(cfg.ParameterSpecs) == 0 && len(cfg.OutputMappings) == 0
}

func writeValidationOnlyModelInvokeResponse(output io.Writer, modelName, operation string) error {
	return json.NewEncoder(output).Encode(modelInvocationValidationResponse{
		ModelName:         modelName,
		Operation:         operation,
		Mode:              modelInvocationValidationOnlyMode,
		ValidationOnly:    true,
		InferenceExecuted: false,
	})
}

type PullConfig struct {
	Context     context.Context
	ModelName   string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type RemoveConfig struct {
	Context     context.Context
	ModelName   string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

// Service exposes the Models CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Inspect(InspectConfig) error
	Invoke(InvokeConfig) error
	Pull(PullConfig) error
	Remove(RemoveConfig) error
}

type httpService struct {
	http             clihttp.Protocol
	pullHTTP         clihttp.Protocol
	invocation       InvocationOperation
	now              func() time.Time
	models           modelinference.Service
	openCatalogScope func(context.Context) (InvokeRuntimeScope, error)
	openInvokeScope  func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	inputFileReader  InputFileReader
}

// New constructs the composition-stable Models CLI service injected into Cobra
// composition. It is a thin facade over the owned adapter Service built from
// composition collaborators when a Models root is available, with HTTP and
// bootstrap invoke behavior retained for remote and legacy composition paths.
func New(
	httpProtocol clihttp.Protocol,
	invocation InvocationOperation,
	providers ...CompositionScopeProvider,
) Service {
	return NewWithOutputFileSystem(httpProtocol, invocation, nil, providers...)
}

// NewWithOutputFileSystem constructs the composition-stable Models CLI
// service with the exact filesystem effect used by explicit generic output
// mappings.
func NewWithOutputFileSystem(
	httpProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	providers ...CompositionScopeProvider,
) Service {
	return NewWithOutputFileSystemAndPullProtocol(
		httpProtocol, httpProtocol, invocation, outputFileSystem, providers...,
	)
}

// NewWithOutputFileSystemAndPullProtocol constructs the Models CLI service
// with an operation-appropriate HTTP protocol for synchronous pulls and
// remote generic inference. The long-running protocol is expected to rely on
// caller cancellation rather than the ordinary short CLI request deadline.
func NewWithOutputFileSystemAndPullProtocol(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	providers ...CompositionScopeProvider,
) Service {
	return NewWithOutputFileSystemAndPullProtocolAndClock(
		httpProtocol, pullHTTPProtocol, invocation, outputFileSystem, nil, providers...,
	)
}

// NewWithOutputFileSystemAndPullProtocolAndClock constructs the Models CLI
// service with the clock used for long-pull progress elapsed time.
func NewWithOutputFileSystemAndPullProtocolAndClock(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	now func() time.Time,
	providers ...CompositionScopeProvider,
) Service {
	return NewWithOutputFileSystemAndPullProtocolAndClockAndInputFileReader(
		httpProtocol, pullHTTPProtocol, invocation, outputFileSystem, now, nil, providers...,
	)
}

// NewWithOutputFileSystemAndPullProtocolAndClockAndInputFileReader constructs
// the Models CLI service with the host effect used by explicit generic input
// mappings.
func NewWithOutputFileSystemAndPullProtocolAndClockAndInputFileReader(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	now func() time.Time,
	inputFileReader InputFileReader,
	providers ...CompositionScopeProvider,
) Service {
	return bindCompositionService(
		httpProtocol, pullHTTPProtocol, invocation, outputFileSystem, inputFileReader, now, providers...,
	)
}

func (service *httpService) List(cfg ListConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	response, err := queryList(queryOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        service.http,
	})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	return renderList(response, cfg.Output)
}

func (service *httpService) Inspect(cfg InspectConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	model, err := queryModel(queryOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		ModelName:   cfg.ModelName,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        service.http,
	})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(model)
	}
	return renderModel(model, cfg.Output)
}

func (service *httpService) Invoke(cfg InvokeConfig) error {
	if err := validateHTTPInvokeRequest(cfg); err != nil {
		return err
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	operation, err := resolveHTTPInvokeOperation(cfg, modelName)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(cfg.Text)
	if err := validateHTTPInvokeText(cfg, text); err != nil {
		return err
	}
	if err := validateHTTPInvokeBindings(cfg); err != nil {
		return err
	}
	if hasGenericCLIInputs(cfg) {
		return service.invokeRemoteGeneric(cfg, modelName, operation)
	}
	return service.invokeLegacyModel(cfg, modelName, operation, text)
}

func validateHTTPInvokeRequest(cfg InvokeConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	return nil
}

func resolveHTTPInvokeOperation(cfg InvokeConfig, modelName string) (string, error) {
	operation := strings.TrimSpace(cfg.Operation)
	if operation == "" && hasGenericCLIInputs(cfg) {
		operation = inferGenericCLIModelOperation(modelName)
	}
	if operation == "" {
		return "", fmt.Errorf("--operation is required")
	}
	return operation, nil
}

func validateHTTPInvokeText(cfg InvokeConfig, text string) error {
	if text == "" && !hasGenericCLIInputs(cfg) {
		return fmt.Errorf("--text is required")
	}
	if text != "" && hasGenericCLIInputs(cfg) {
		return clidiag.NewFlagConflictFailure(
			"--text", "--input", fmt.Errorf("choose one input form for model invocation"),
		)
	}
	return nil
}

func hasGenericCLIInputs(cfg InvokeConfig) bool {
	return len(cfg.InputMappings) > 0 || len(cfg.InputSpecs) > 0
}

func (service *httpService) invokeLegacyModel(
	cfg InvokeConfig,
	modelName string,
	operation string,
	text string,
) error {
	if validationOnlyModelInvoke(cfg) {
		if err := service.validateModelInvoke(cfg, modelName, operation); err != nil {
			return err
		}
		return writeValidationOnlyModelInvokeResponse(cfg.Output, modelName, operation)
	}

	outputPath := strings.TrimSpace(cfg.OutputPath)
	if outputPath == "" {
		return fmt.Errorf("--output is required unless --json is set")
	}
	if err := invokeModelAudio(invokeOptions{
		Context:          cfg.Context,
		Server:           cfg.Server,
		ModelName:        modelName,
		Operation:        operation,
		Text:             text,
		OutputPath:       outputPath,
		FactoryDir:       cfg.FactoryDir,
		WorkingDirectory: cfg.WorkingDirectory,
		HomeDir:          cfg.HomeDir,
		OperatorDefaults: cfg.OperatorDefaults,
		Logger:           cfg.Logger,
		Verbose:          cfg.Verbose,
		Diagnostics:      cfg.Diagnostics,
		Invocation:       service.invocation,
	}); err != nil {
		return err
	}
	_, err := fmt.Fprintf(cfg.Output, "Wrote audio: %s\n", outputPath)
	return err
}

func validateHTTPInvokeBindings(cfg InvokeConfig) error {
	switch {
	case len(cfg.ParameterSpecs) > 0:
		return fmt.Errorf("explicit generic parameters require the local Models composition")
	case len(cfg.OutputMappings) > 0:
		return fmt.Errorf("explicit output mappings require the local Models composition")
	default:
		return nil
	}
}

func (service *httpService) Pull(cfg PullConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	pullHTTP := service.pullHTTP
	if pullHTTP == nil {
		pullHTTP = service.http
	}
	response, err := pullModel(pullOptions{
		Context:          cfg.Context,
		Server:           cfg.Server,
		ModelName:        modelName,
		Verbose:          cfg.Verbose,
		Diagnostics:      cfg.Diagnostics,
		HTTP:             pullHTTP,
		ProgressInterval: modelPullProgressInterval,
		Now:              service.now,
	})
	if cfg.JSON {
		if encodeErr := json.NewEncoder(cfg.Output).Encode(response); encodeErr != nil {
			return encodeErr
		}
	}
	if err != nil {
		return err
	}
	if cfg.JSON {
		return nil
	}
	return renderPull(response, cfg.Output)
}

func (service *httpService) Remove(cfg RemoveConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	response, err := removeModel(removeOptions{
		Context: cfg.Context, Server: cfg.Server, ModelName: modelName,
		Verbose: cfg.Verbose, Diagnostics: cfg.Diagnostics, HTTP: service.http,
	})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	return renderRemove(response, cfg.Output)
}

type queryOptions struct {
	Context     context.Context
	Server      string
	ModelName   string
	Verbose     bool
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func queryList(cfg queryOptions) (factoryapi.ListModelsResponse, error) {
	endpoint, err := modelsEndpoint(cfg.Server, "/models")
	if err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	var response factoryapi.ListModelsResponse
	if err := doModelsGET(cfg.Context, cfg.HTTP, endpoint, &response, requestDiagnostics{
		Enabled:     cfg.Verbose,
		Output:      cfg.Diagnostics,
		Command:     "models list",
		Server:      cfg.Server,
		SummaryFunc: func() string { return fmt.Sprintf("resultCount=%d", len(response.Results)) },
	}); err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	return response, nil
}

func queryModelWithProtocol(ctx context.Context, transport clihttp.Protocol, server, modelName string) (factoryapi.ModelDetail, error) {
	return queryModel(queryOptions{Context: ctx, HTTP: transport, Server: server, ModelName: modelName})
}

func queryModel(cfg queryOptions) (factoryapi.ModelDetail, error) {
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName))
	endpoint, err := modelsEndpoint(cfg.Server, path)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	var response factoryapi.ModelDetail
	if err := doModelsGET(cfg.Context, cfg.HTTP, endpoint, &response, requestDiagnostics{
		Enabled:   cfg.Verbose,
		Output:    cfg.Diagnostics,
		Command:   "models inspect",
		Server:    cfg.Server,
		ModelName: strings.TrimSpace(cfg.ModelName),
		SummaryFunc: func() string {
			return fmt.Sprintf(
				"readiness=%s lifecycle=%s operations=%d",
				managedRuntimeReadiness(response.ManagedRuntime),
				managedRuntimeLifecycle(response.ManagedRuntime),
				len(response.Operations),
			)
		},
	}); err != nil {
		return factoryapi.ModelDetail{}, err
	}
	return response, nil
}

type invokeOptions struct {
	Context          context.Context
	Server           string
	ModelName        string
	Operation        string
	Text             string
	OutputPath       string
	FactoryDir       string
	WorkingDirectory string
	HomeDir          string
	OperatorDefaults operatorconfig.ResolvedDefaults
	Logger           *zap.Logger
	Verbose          bool
	Diagnostics      io.Writer
	Invocation       InvocationOperation
}

// invokeModelAudio copies streamed audio from the bootstrap-owned invocation
// result instead of requiring a listening factory API HTTP server.
func invokeModelAudio(cfg invokeOptions) error {
	mode := factoryapi.ModelInvocationResponseMode("AUDIO_STREAM")
	result, err := invokeModelThroughBootstrap(cfg, &mode)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.StreamFile) == "" {
		return fmt.Errorf("models invoke bootstrap returned no streamed audio output")
	}
	if err := cfg.Invocation.ExportModelInvocationArtifact(result.StreamFile, cfg.OutputPath); err != nil {
		return err
	}
	logBootstrapInvokeResponse(cfg, fmt.Sprintf("outputPath=%s", cfg.OutputPath))
	return nil
}

type pullOptions struct {
	Context          context.Context
	Server           string
	ModelName        string
	Verbose          bool
	Diagnostics      io.Writer
	HTTP             clihttp.Protocol
	ProgressInterval time.Duration
	Now              func() time.Time
}

func pullModel(cfg pullOptions) (factoryapi.ModelPullResponse, error) {
	var response factoryapi.ModelPullResponse
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName)) + "/pull"
	diagnostics := newSynchronizedWriter(cfg.Diagnostics)
	stopProgress := startPullProgress(
		cfg.Context, cfg.ModelName, diagnostics, cfg.ProgressInterval, cfg.Now,
	)
	defer stopProgress()
	err := doModelsPOST(cfg.Context, cfg.HTTP, cfg.Server, path, map[string]any{}, &response, requestDiagnostics{
		Enabled:   cfg.Verbose,
		Output:    diagnostics,
		Command:   "models pull",
		Server:    cfg.Server,
		ModelName: strings.TrimSpace(cfg.ModelName),
		SummaryFunc: func() string {
			return fmt.Sprintf(
				"pullOutcome=%s readiness=%s downloadedFiles=%d",
				response.ManagedRuntimePull.PullOutcome,
				response.ManagedRuntimePull.ReadinessState,
				len(response.DownloadedFiles),
			)
		},
	})
	projectPullResponseOutcome(&response)
	if err != nil {
		return response, err
	}
	return response, nil
}

func projectPullResponseOutcome(response *factoryapi.ModelPullResponse) {
	if response == nil || strings.TrimSpace(string(response.ManagedRuntimePull.PullOutcome)) == "" {
		return
	}
	response.Outcome = modelPullOutcomeFromManagedRuntime(response.ManagedRuntimePull.PullOutcome)
}

type requestDiagnostics struct {
	Enabled      bool
	Output       io.Writer
	Command      string
	Server       string
	ModelName    string
	Operation    string
	OutputPath   string
	RequestBytes int
	SummaryFunc  func() string
}

func doModelsPOST(ctx context.Context, transport clihttp.Protocol, server, path string, payload any, out any, diagnostics requestDiagnostics) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if transport == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal models request: %w", err)
	}
	endpoint, err := modelsEndpoint(server, path)
	if err != nil {
		return err
	}
	diagnostics.RequestBytes = len(body)
	logModelsRequest(diagnostics, endpoint)

	response, err := transport.PostJSON(
		ctx,
		endpoint.String(),
		bytes.NewReader(body),
		out,
	)
	if err != nil {
		logModelsResponse(
			diagnostics, endpoint, 0, response.Duration,
			modelsTransportErrorSummary(err),
		)
		if response.HTTP != nil {
			return malformedModelsResponseError(err)
		}
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	if resp == nil || resp.Body == nil {
		return malformedModelsResponseError(nil)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return clihttp.WithHTTPResponse(resp, fmt.Errorf("read models response: %w", readErr))
		}
		logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d", len(responseBody)))
		if pullErr := managedRuntimePullResponseError(resp.StatusCode, responseBody); pullErr != nil {
			if out != nil {
				if err := json.Unmarshal(responseBody, out); err != nil {
					return clihttp.WithHTTPResponse(resp, fmt.Errorf("decode managed runtime pull response: %w", err))
				}
				sanitizeManagedRuntimePullResponse(out)
			}
			return clihttp.WithHTTPResponse(resp, pullErr)
		}
		return modelsRequestError(resp.StatusCode, responseBody, resp)
	}
	responseBytes, err := modelsResponseBytes(out)
	if err != nil {
		return err
	}
	sanitizeManagedRuntimePullResponse(out)
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d %s", responseBytes, diagnostics.summary()))
	return nil
}

func sanitizeManagedRuntimePullResponse(out any) {
	response, ok := out.(*factoryapi.ModelPullResponse)
	if !ok || response == nil || response.ManagedRuntimePull.PullDiagnostics == nil {
		return
	}
	diagnosticsError := managedRuntimePullDiagnosticsFromGenerated(response.ManagedRuntimePull.PullDiagnostics)
	diagnostics := pullsupport.PullDiagnosticsFromError(diagnosticsError)
	response.ManagedRuntimePull.PullDiagnostics = managedRuntimePullDiagnostics(
		modelinference.PullResult{PullDiagnostics: diagnostics},
	)
}

func doModelsGET(ctx context.Context, transport clihttp.Protocol, endpoint url.URL, out any, diagnostics requestDiagnostics) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if transport == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	logModelsRequest(diagnostics, endpoint)
	response, err := transport.GetJSON(
		ctx,
		endpoint.String(),
		out,
	)
	if err != nil {
		logModelsResponse(
			diagnostics, endpoint, 0, response.Duration,
			modelsTransportErrorSummary(err),
		)
		if response.HTTP != nil {
			return malformedModelsResponseError(err)
		}
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	if resp == nil || resp.Body == nil {
		return malformedModelsResponseError(nil)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return clihttp.WithHTTPResponse(resp, fmt.Errorf("read models response: %w", err))
		}
		logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d", len(body)))
		return modelsRequestError(resp.StatusCode, body, resp)
	}
	responseBytes, err := modelsResponseBytes(out)
	if err != nil {
		return err
	}
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d %s", responseBytes, diagnostics.summary()))
	return nil
}

func modelsResponseBytes(out any) (int, error) {
	body, err := json.Marshal(out)
	if err != nil {
		return 0, fmt.Errorf("marshal models response: %w", err)
	}
	return len(body), nil
}

func modelsTransportErrorSummary(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "error=timeout"
	case errors.Is(err, context.Canceled):
		return "error=canceled"
	default:
		return "error=unreachable"
	}
}

func (diagnostics requestDiagnostics) summary() string {
	if diagnostics.SummaryFunc == nil {
		return ""
	}
	return strings.TrimSpace(diagnostics.SummaryFunc())
}

func logModelsRequest(diagnostics requestDiagnostics, endpoint url.URL) {
	clidiag.Printf(
		diagnostics.Output,
		diagnostics.Enabled,
		"%s request endpointPath=%s server=%s modelName=%q operation=%q outputPath=%s requestBytes=%d",
		diagnostics.Command,
		endpoint.Path,
		safeModelsServerLabel(endpoint),
		diagnostics.ModelName,
		diagnostics.Operation,
		diagnostics.OutputPath,
		diagnostics.RequestBytes,
	)
}

func safeModelsServerLabel(endpoint url.URL) string {
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return "<unknown>"
	}
	return endpoint.Scheme + "://" + endpoint.Host
}

func logModelsResponse(diagnostics requestDiagnostics, endpoint url.URL, statusCode int, elapsed time.Duration, summary string) {
	if strings.TrimSpace(summary) == "" {
		clidiag.Printf(diagnostics.Output, diagnostics.Enabled, "%s response endpointPath=%s status=%d durationMillis=%d", diagnostics.Command, endpoint.Path, statusCode, elapsed.Milliseconds())
		return
	}
	clidiag.Printf(diagnostics.Output, diagnostics.Enabled, "%s response endpointPath=%s status=%d durationMillis=%d %s", diagnostics.Command, endpoint.Path, statusCode, elapsed.Milliseconds(), summary)
}

const remoteModelsInvokeScope = "models-cli:remote"

func (service *httpService) invokeRemoteGeneric(
	cfg InvokeConfig,
	modelName string,
	operation string,
) error {
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models invoke")
	}
	// Catalog discovery is a short control-plane request, but generic
	// inference can include model loading and backend execution. Use the
	// caller-cancellable long-running protocol for the POST so the ordinary
	// CLI control-plane deadline cannot cancel a real remote invocation.
	inferenceHTTP := service.pullHTTP
	if inferenceHTTP == nil {
		inferenceHTTP = service.http
	}
	inputFileReader, err := preflightGenericCLIInputsWithReader(cfg, service.inputFileReader)
	if err != nil {
		return err
	}
	model, err := queryModel(queryOptions{
		Context: cfg.Context, Server: cfg.Server, ModelName: modelName,
		Verbose: cfg.Verbose, Diagnostics: cfg.Diagnostics, HTTP: service.http,
	})
	if err != nil {
		return err
	}
	catalog := genericCLIModelDetailFromGenerated(model)
	if err := validateCLIOutputShape(cfg, catalog, operation); err != nil {
		return err
	}
	inputs, err := prepareGenericCLIInputsWithReader(cfg, operation, catalog, inputFileReader)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.OutputPath) != "" {
		return fmt.Errorf("--output is not supported with explicit generic inputs")
	}
	request := genericCLIRequestFromInputs(modelName, operation, inputs)
	var response factoryapi.GenericModelInvocationResponse
	if err := doModelsPOST(
		cfg.Context, inferenceHTTP, cfg.Server, "/models/invocations", request, &response,
		requestDiagnostics{
			Enabled: cfg.Verbose, Output: cfg.Diagnostics, Command: "models invoke",
			Server: cfg.Server, ModelName: modelName, Operation: operation,
		},
	); err != nil {
		return err
	}
	if response.Failure != nil {
		return genericCLIInvocationFailureFromGenerated(response.Failure, modelName, operation)
	}
	if err := validateGenericCLIResponse(response); err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	result, err := genericCLIResultFromGenerated(response, modelName, operation)
	if err != nil {
		return malformedModelsResponseError(err)
	}
	return writeGenericCLIOutputWithCatalog(cfg.Output, result, catalog, operation)
}
func genericCLIInvocationFailureFromGenerated(
	failure *factoryapi.ModelInvocationFailure,
	modelName string,
	operation string,
) error {
	if failure == nil {
		return nil
	}
	if strings.TrimSpace(string(failure.Class)) == "" || strings.TrimSpace(failure.Message) == "" {
		return malformedModelsResponseError(nil)
	}
	mapped := &modelinference.InvocationFailure{
		Class:     modelinference.InvocationFailureClass(failure.Class),
		Message:   strings.TrimSpace(failure.Message),
		Model:     modelinference.ModelReference{NameOrURI: modelName},
		Operation: operation,
	}
	if failure.Model != nil && strings.TrimSpace(failure.Model.NameOrUri) != "" {
		mapped.Model = modelinference.ModelReference{NameOrURI: failure.Model.NameOrUri}
	}
	if failure.Slot != nil {
		mapped.Slot = strings.TrimSpace(*failure.Slot)
	}
	if failure.Parameter != nil {
		mapped.Parameter = strings.TrimSpace(*failure.Parameter)
	}
	if failure.Field != nil {
		mapped.Field = strings.TrimSpace(*failure.Field)
	}
	if failure.Operation != nil && strings.TrimSpace(*failure.Operation) != "" {
		mapped.Operation = strings.TrimSpace(*failure.Operation)
	}
	return mapModelsClientError(mapped)
}
func validateGenericCLIResponse(response factoryapi.GenericModelInvocationResponse) error {
	if len(response.Outputs) == 0 {
		return malformedModelsResponseError(nil)
	}
	for _, output := range response.Outputs {
		if strings.TrimSpace(output.Name) == "" || !genericCLIResponseModalityValid(output.Modality) {
			return malformedModelsResponseError(nil)
		}
		if output.Content == nil && output.Artifact == nil {
			return malformedModelsResponseError(nil)
		}
		if output.Content != nil && *output.Content == "" && output.Artifact == nil {
			return malformedModelsResponseError(nil)
		}
		if output.Artifact != nil && strings.TrimSpace(output.Artifact.ArtifactRef) == "" {
			return malformedModelsResponseError(nil)
		}
	}
	return nil
}
func genericCLIResponseModalityValid(modality factoryapi.ModelInvocationContentType) bool {
	switch modality {
	case factoryapi.ModelInvocationContentTypeText,
		factoryapi.ModelInvocationContentTypeImage,
		factoryapi.ModelInvocationContentTypeAudio,
		factoryapi.ModelInvocationContentTypeVideo,
		factoryapi.ModelInvocationContentTypeJSON,
		factoryapi.ModelInvocationContentTypeBinary:
		return true
	default:
		return false
	}
}
func genericCLIRequestFromInputs(
	modelName string,
	operation string,
	inputs []modelinference.InferenceInput,
) factoryapi.GenericModelInvocationRequest {
	generatedInputs := make([]factoryapi.ModelInvocationInput, len(inputs))
	for index, input := range inputs {
		generated := factoryapi.ModelInvocationInput{
			Name:     input.Name,
			Modality: factoryapi.ModelInvocationContentType(input.Modality),
		}
		generated.ContentType = genericCLIStringPointer(input.ContentType)
		generated.MediaType = genericCLIStringPointer(input.MediaType)
		if input.Artifact != nil && !input.Artifact.IsZero() {
			artifactRef := input.Artifact.String()
			generated.ArtifactRef = &artifactRef
		} else if genericCLIInputUsesBinaryCarrier(input.Modality) {
			content := []byte(input.Content)
			generated.ContentBase64 = &content
		} else {
			generated.Content = genericCLIStringPointer(input.Content)
		}
		generatedInputs[index] = generated
	}
	operationValue := operation
	return factoryapi.GenericModelInvocationRequest{
		Scope:     remoteModelsInvokeScope,
		Holder:    modelsCLIInvokeHolder,
		Model:     factoryapi.ModelReference{NameOrUri: modelName},
		Operation: &operationValue,
		Inputs:    &generatedInputs,
	}
}
func genericCLIInputUsesBinaryCarrier(modality modelinference.Modality) bool {
	switch modality {
	case modelinference.ModalityAudio, modelinference.ModalityImage,
		modelinference.ModalityVideo, modelinference.ModalityBinary:
		return true
	default:
		return false
	}
}
