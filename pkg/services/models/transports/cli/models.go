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
	"sort"
	"strings"
	"time"

	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
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
	Context          context.Context
	ModelName        string
	Operation        string
	Text             string
	OutputPath       string
	OutputMappings   []string
	Server           string
	FactoryDir       string
	HomeDir          string
	OperatorDefaults operatorconfig.ResolvedDefaults
	Logger           *zap.Logger
	JSON             bool
	Verbose          bool
	Debug            bool
	Output           io.Writer
	Diagnostics      io.Writer
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
	http       clihttp.Protocol
	pullHTTP   clihttp.Protocol
	invocation InvocationOperation
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
// with an operation-appropriate HTTP protocol for synchronous pulls. The
// pull protocol is expected to rely on caller cancellation rather than the
// ordinary short CLI request deadline.
func NewWithOutputFileSystemAndPullProtocol(
	httpProtocol clihttp.Protocol,
	pullHTTPProtocol clihttp.Protocol,
	invocation InvocationOperation,
	outputFileSystem OutputFileSystem,
	providers ...CompositionScopeProvider,
) Service {
	return bindCompositionService(
		httpProtocol, pullHTTPProtocol, invocation, outputFileSystem, providers...,
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
	operation := strings.TrimSpace(cfg.Operation)
	if operation == "" {
		return fmt.Errorf("--operation is required")
	}
	text := strings.TrimSpace(cfg.Text)
	if text == "" {
		return fmt.Errorf("--text is required")
	}
	if len(cfg.OutputMappings) > 0 {
		return fmt.Errorf("explicit output mappings require the local Models composition")
	}

	if cfg.JSON {
		response, err := invokeModelMetadata(invokeOptions{
			Context:          cfg.Context,
			Server:           cfg.Server,
			ModelName:        modelName,
			Operation:        operation,
			Text:             text,
			FactoryDir:       cfg.FactoryDir,
			HomeDir:          cfg.HomeDir,
			OperatorDefaults: cfg.OperatorDefaults,
			Logger:           cfg.Logger,
			Verbose:          cfg.Verbose,
			Diagnostics:      cfg.Diagnostics,
			Invocation:       service.invocation,
		})
		if err != nil {
			return err
		}
		return json.NewEncoder(cfg.Output).Encode(response)
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
		Context:     cfg.Context,
		Server:      cfg.Server,
		ModelName:   modelName,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        pullHTTP,
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
	HomeDir          string
	OperatorDefaults operatorconfig.ResolvedDefaults
	Logger           *zap.Logger
	Verbose          bool
	Diagnostics      io.Writer
	Invocation       InvocationOperation
}

func invokeModelMetadata(cfg invokeOptions) (factoryapi.ModelInvocationResponse, error) {
	result, err := invokeModelThroughBootstrap(cfg, nil)
	if err != nil {
		return factoryapi.ModelInvocationResponse{}, err
	}
	response := modelInvocationResponseFromResult(result)
	logBootstrapInvokeResponse(cfg, fmt.Sprintf("worker=%s contentParts=%d", response.Worker, len(response.Content)))
	return response, nil
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
	Context     context.Context
	Server      string
	ModelName   string
	Verbose     bool
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func pullModel(cfg pullOptions) (factoryapi.ModelPullResponse, error) {
	var response factoryapi.ModelPullResponse
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName)) + "/pull"
	if err := doModelsPOST(cfg.Context, cfg.HTTP, cfg.Server, path, map[string]any{}, &response, requestDiagnostics{
		Enabled:   cfg.Verbose,
		Output:    cfg.Diagnostics,
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
	}); err != nil {
		return response, err
	}
	return response, nil
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
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
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
			}
			return clihttp.WithHTTPResponse(resp, pullErr)
		}
		return modelsRequestError(resp.StatusCode, responseBody, resp)
	}
	responseBytes, err := modelsResponseBytes(out)
	if err != nil {
		return err
	}
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d %s", responseBytes, diagnostics.summary()))
	return nil
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
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
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
		"%s request endpointPath=%s endpoint=%s server=%s modelName=%q operation=%q outputPath=%s requestBytes=%d",
		diagnostics.Command,
		endpoint.Path,
		endpoint.String(),
		diagnostics.Server,
		diagnostics.ModelName,
		diagnostics.Operation,
		diagnostics.OutputPath,
		diagnostics.RequestBytes,
	)
}

func logModelsResponse(diagnostics requestDiagnostics, endpoint url.URL, statusCode int, elapsed time.Duration, summary string) {
	if strings.TrimSpace(summary) == "" {
		clidiag.Printf(diagnostics.Output, diagnostics.Enabled, "%s response endpointPath=%s status=%d durationMillis=%d", diagnostics.Command, endpoint.Path, statusCode, elapsed.Milliseconds())
		return
	}
	clidiag.Printf(diagnostics.Output, diagnostics.Enabled, "%s response endpointPath=%s status=%d durationMillis=%d %s", diagnostics.Command, endpoint.Path, statusCode, elapsed.Milliseconds(), summary)
}

func renderList(response factoryapi.ListModelsResponse, output io.Writer) error {
	if _, err := fmt.Fprintln(output, "NAME\tREADINESS\tLIFECYCLE\tLOCALITY\tOPERATIONS\tMODALITIES\tRESOURCES\tCACHE SIZE"); err != nil {
		return err
	}
	results := append([]factoryapi.ModelSummary(nil), response.Results...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	for _, model := range results {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\n",
			model.Name,
			managedRuntimeReadiness(model.ManagedRuntime),
			managedRuntimeLifecycle(model.ManagedRuntime),
			model.ProviderLocality,
			modelOperationNames(model.Operations),
			modelModalities(model.Modalities),
			len(model.Resources),
			managedRuntimeCacheSize(model.ManagedRuntime),
		); err != nil {
			return err
		}
	}
	return nil
}

func managedRuntimeCacheSize(runtime factoryapi.ManagedRuntime) string {
	if runtime.CacheBytes == nil || *runtime.CacheBytes < 0 {
		return "NOT_INSTALLED"
	}
	return fmt.Sprintf("%s (%d bytes)", humanByteSize(*runtime.CacheBytes), *runtime.CacheBytes)
}

func managedRuntimeRevision(runtime factoryapi.ManagedRuntime) string {
	if runtime.Revision == nil || strings.TrimSpace(*runtime.Revision) == "" {
		return "NOT_INSTALLED"
	}
	return *runtime.Revision
}

func managedRuntimeCachePath(runtime factoryapi.ManagedRuntime) string {
	if runtime.CachePath == nil || strings.TrimSpace(*runtime.CachePath) == "" {
		return "NOT_INSTALLED"
	}
	return *runtime.CachePath
}

func humanByteSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	for _, unit := range units {
		value /= 1024
		if value < 1024 || unit == units[len(units)-1] {
			return fmt.Sprintf("%.2f %s", value, unit)
		}
	}
	return fmt.Sprintf("%d B", bytes)
}

func renderPull(response factoryapi.ModelPullResponse, output io.Writer) error {
	if _, err := fmt.Fprintf(
		output,
		"MODEL\tPULL OUTCOME\tREADINESS\tLIFECYCLE\tREVISION\tCACHE PATH\n%s\t%s\t%s\t%s\t%s\t%s\n",
		response.ModelName,
		response.ManagedRuntimePull.PullOutcome,
		response.ManagedRuntimePull.ReadinessState,
		managedRuntimeLifecycleFromPull(response),
		response.Revision,
		response.CachePath,
	); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "FILES"); err != nil {
		return err
	}
	files := append([]factoryapi.ModelPullDownloadedFile(nil), response.DownloadedFiles...)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	for _, file := range files {
		if _, err := fmt.Fprintf(output, "%s\t%d\n", file.Path, file.Bytes); err != nil {
			return err
		}
	}
	return nil
}

func renderModel(model factoryapi.ModelDetail, output io.Writer) error {
	if _, err := fmt.Fprintf(output, "Name:\t%s\n", model.Name); err != nil {
		return err
	}
	for _, row := range []struct {
		label string
		value string
	}{
		{label: "Readiness", value: managedRuntimeReadiness(model.ManagedRuntime)},
		{label: "Lifecycle", value: managedRuntimeLifecycle(model.ManagedRuntime)},
		{label: "Locality", value: string(model.ProviderLocality)},
		{label: "Revision", value: managedRuntimeRevision(model.ManagedRuntime)},
		{label: "Cache Size", value: managedRuntimeCacheSize(model.ManagedRuntime)},
		{label: "Cache Path", value: managedRuntimeCachePath(model.ManagedRuntime)},
		{label: "Operations", value: modelOperationNames(model.Operations)},
		{label: "Modalities", value: modelModalities(model.Modalities)},
	} {
		if _, err := fmt.Fprintf(output, "%s:\t%s\n", row.label, row.value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "Resources:\t%d\n", len(model.Resources)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "Capabilities:"); err != nil {
		return err
	}
	for _, capability := range model.Capabilities {
		if _, err := fmt.Fprintf(
			output,
			"- %s\t%s\t%s\n",
			capability.Worker,
			capability.ProviderLocality,
			modelOperationNames(capability.Operations),
		); err != nil {
			return err
		}
	}
	diagnostics := managedRuntimeDiagnosticsMap(model)
	if len(diagnostics) > 0 {
		keys := make([]string, 0, len(diagnostics))
		for key := range diagnostics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(output, "Diagnostics:"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(output, "- %s=%s\n", key, diagnostics[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func managedRuntimeReadiness(runtime factoryapi.ManagedRuntime) string {
	if strings.TrimSpace(string(runtime.ReadinessState)) == "" {
		return "UNKNOWN"
	}
	return string(runtime.ReadinessState)
}

func managedRuntimeLifecycle(runtime factoryapi.ManagedRuntime) string {
	if strings.TrimSpace(string(runtime.LifecycleState)) == "" {
		return "UNKNOWN"
	}
	return string(runtime.LifecycleState)
}

func managedRuntimeLifecycleFromPull(response factoryapi.ModelPullResponse) string {
	switch response.ManagedRuntimePull.PullOutcome {
	case factoryapi.ManagedRuntimePullOutcomeSTILLLOADING:
		return string(factoryapi.ManagedRuntimeLifecycleStateINSTALLING)
	case factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY,
		factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT,
		factoryapi.ManagedRuntimePullOutcomeALREADYREADY:
		return string(factoryapi.ManagedRuntimeLifecycleStateINSTALLED)
	default:
		return "UNKNOWN"
	}
}

func managedRuntimeDiagnosticsMap(model factoryapi.ModelDetail) factoryapi.StringMap {
	if model.ManagedRuntime.Diagnostics != nil && len(*model.ManagedRuntime.Diagnostics) > 0 {
		return *model.ManagedRuntime.Diagnostics
	}
	return model.Diagnostics
}

func modelOperationNames(operations []factoryapi.ModelInvocationOperation) string {
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func modelModalities(modalities []factoryapi.ModelInvocationContentType) string {
	values := make([]string, 0, len(modalities))
	for _, modality := range modalities {
		values = append(values, string(modality))
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func managedRuntimePullResponseError(statusCode int, body []byte) error {
	if statusCode != http.StatusUnprocessableEntity && statusCode != http.StatusGatewayTimeout {
		return nil
	}
	var response factoryapi.ModelPullResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil
	}
	switch response.ManagedRuntimePull.PullOutcome {
	case factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED,
		factoryapi.ManagedRuntimePullOutcomeTIMEDOUT,
		factoryapi.ManagedRuntimePullOutcomeSTILLLOADING:
		return &managedRuntimePullFailure{
			Outcome:   response.ManagedRuntimePull.PullOutcome,
			Readiness: response.ManagedRuntimePull.ReadinessState,
		}
	default:
		return nil
	}
}

type managedRuntimePullFailure struct {
	Outcome   factoryapi.ManagedRuntimePullOutcome
	Readiness factoryapi.ManagedRuntimeReadinessState
}

func (failure *managedRuntimePullFailure) Error() string {
	if failure == nil {
		return ""
	}
	return fmt.Sprintf(
		"managed runtime pull failed (pullOutcome=%s readinessState=%s)",
		failure.Outcome,
		failure.Readiness,
	)
}

func (failure *managedRuntimePullFailure) CLIErrorCode() string {
	return managedRuntimePullFailureCode
}

func (failure *managedRuntimePullFailure) CLIErrorFamily() factoryapi.ErrorFamily {
	return factoryapi.ErrorFamilyBadRequest
}

func (failure *managedRuntimePullFailure) CLIErrorMessage() string {
	return failure.Error()
}

func modelsRequestError(statusCode int, body []byte, response ...*http.Response) error {
	var httpResponse *http.Response
	if len(response) > 0 {
		httpResponse = response[0]
	}
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		switch errResp.Code {
		case factoryapi.ErrorResponseCodeMODELCACHENOTFOUND:
			return fmt.Errorf("%w: %s", ErrModelCacheNotFound, errResp.Message)
		case factoryapi.ErrorResponseCodeMODELCACHEINUSE:
			return fmt.Errorf("%w: %s", ErrModelCacheInUse, errResp.Message)
		}
		if statusCode == http.StatusNotFound && errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
			if httpResponse != nil {
				return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, fmt.Sprintf("%s: %s", ErrModelNotFound, errResp.Message), ErrModelNotFound)
			}
			return clihttp.NewAPIError(statusCode, errResp, fmt.Sprintf("%s: %s", ErrModelNotFound, errResp.Message), ErrModelNotFound)
		}
		if httpResponse != nil {
			return clihttp.NewAPIErrorFromResponse(httpResponse, errResp, fmt.Sprintf("models request failed (%d): %s", statusCode, errResp.Message), nil)
		}
		return clihttp.NewAPIError(statusCode, errResp, fmt.Sprintf("models request failed (%d): %s", statusCode, errResp.Message), nil)
	}
	return clihttp.WithHTTPResponse(
		httpResponse,
		fmt.Errorf("models request failed (%d): response body was not a structured API error", statusCode),
	)
}

func modelsEndpoint(server, path string) (url.URL, error) {
	endpointURL, err := cliserver.RequestURL(server, path)
	if err != nil {
		return url.URL{}, err
	}
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return url.URL{}, fmt.Errorf("parse models endpoint: %w", err)
	}
	return *endpoint, nil
}

func mustGeneratedTextContentPart(text string) factoryapi.WorkContentPart {
	var part factoryapi.WorkContentPart
	slot := "text"
	_ = part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
		Slot: &slot,
	})
	return part
}
