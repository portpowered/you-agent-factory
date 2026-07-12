// Package models defines model-discovery command behavior.
package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/cli/clihttp"
	"github.com/portpowered/infinite-you/pkg/cli/cliserver"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/localmodels"
)

const (
	modelsRequestTimeout       = 10 * time.Second
	modelsErrorBodyPreviewSize = 200
)

var ErrModelNotFound = errors.New("model not found")

type ListConfig struct {
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type InspectConfig struct {
	ModelName   string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type InvokeConfig struct {
	ModelName   string
	Operation   string
	Text        string
	OutputPath  string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer
}

type PullConfig struct {
	ModelName   string
	Server      string
	JSON        bool
	Verbose     bool
	Debug       bool
	Output      io.Writer
	Diagnostics io.Writer

	// runtimeConfig and assetPuller keep direct pull dependencies explicit for
	// package tests. Production callers leave both unset.
	runtimeConfig *factoryconfig.LoadedFactoryConfig
	assetPuller   localmodels.AssetPuller
}

func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	response, err := queryList(queryOptions{
		Server:      cfg.Server,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	return RenderList(response, cfg.Output)
}

func Inspect(cfg InspectConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	model, err := queryModel(queryOptions{
		Server:      cfg.Server,
		ModelName:   cfg.ModelName,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	})
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(model)
	}
	return RenderModel(model, cfg.Output)
}

func Invoke(cfg InvokeConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
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

	if cfg.JSON {
		response, err := invokeModelMetadata(invokeOptions{
			Server:      cfg.Server,
			ModelName:   modelName,
			Operation:   operation,
			Text:        text,
			Verbose:     cfg.Verbose,
			Diagnostics: cfg.Diagnostics,
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
		Server:      cfg.Server,
		ModelName:   modelName,
		Operation:   operation,
		Text:        text,
		OutputPath:  outputPath,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
	}); err != nil {
		return err
	}
	_, err := fmt.Fprintf(cfg.Output, "Wrote audio: %s\n", outputPath)
	return err
}

func Pull(cfg PullConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	response, err := pullModelDirectly(context.Background(), modelName, cfg.runtimeConfig, cfg.assetPuller)
	logDirectPull(cfg, response, err)
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
	return RenderPull(response, cfg.Output)
}

func pullModelDirectly(
	ctx context.Context,
	modelName string,
	runtimeCfg *factoryconfig.LoadedFactoryConfig,
	puller localmodels.AssetPuller,
) (factoryapi.ModelPullResponse, error) {
	if runtimeCfg == nil {
		loaded, err := factoryconfig.LoadFromCanonicalJSON(factoryconfig.BuiltInTTSFactoryJSON, nil)
		if err != nil {
			return factoryapi.ModelPullResponse{}, fmt.Errorf("load built-in managed model catalog: %w", err)
		}
		runtimeCfg = loaded
	}
	if puller == nil {
		puller = localmodels.NewAssetPuller("")
	}

	result, err := localmodels.PullModelWithOptions(puller, ctx, runtimeCfg, modelName, localmodels.PullOptions{
		RuntimeCacheInspector: puller,
		SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
	})
	return apisurface.ModelPullResponseFromService(result), err
}

func logDirectPull(cfg PullConfig, response factoryapi.ModelPullResponse, err error) {
	if !cfg.Verbose || cfg.Diagnostics == nil {
		return
	}
	if err != nil {
		clidiag.Printf(cfg.Diagnostics, true, "models pull direct modelName=%q outcome=%s readiness=%s error=%t", cfg.ModelName, response.ManagedRuntimePull.PullOutcome, response.ManagedRuntimePull.ReadinessState, true)
		return
	}
	clidiag.Printf(cfg.Diagnostics, true, "models pull direct modelName=%q pullOutcome=%s readiness=%s downloadedFiles=%d", cfg.ModelName, response.ManagedRuntimePull.PullOutcome, response.ManagedRuntimePull.ReadinessState, len(response.DownloadedFiles))
}

type queryOptions struct {
	Server      string
	ModelName   string
	Verbose     bool
	Diagnostics io.Writer
}

func queryList(cfg queryOptions) (factoryapi.ListModelsResponse, error) {
	endpoint, err := modelsEndpoint(cfg.Server, "/models")
	if err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	var response factoryapi.ListModelsResponse
	if err := doModelsGET(endpoint, &response, requestDiagnostics{
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

func QueryModel(server, modelName string) (factoryapi.ModelDetail, error) {
	return queryModel(queryOptions{Server: server, ModelName: modelName})
}

func queryModel(cfg queryOptions) (factoryapi.ModelDetail, error) {
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName))
	endpoint, err := modelsEndpoint(cfg.Server, path)
	if err != nil {
		return factoryapi.ModelDetail{}, err
	}
	var response factoryapi.ModelDetail
	if err := doModelsGET(endpoint, &response, requestDiagnostics{
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
	Server      string
	ModelName   string
	Operation   string
	Text        string
	OutputPath  string
	Verbose     bool
	Diagnostics io.Writer
}

func invokeModelMetadata(cfg invokeOptions) (factoryapi.ModelInvocationResponse, error) {
	request := factoryapi.ModelInvocationRequest{
		Operation: cfg.Operation,
		Content: &factoryapi.WorkContent{
			mustGeneratedTextContentPart(cfg.Text),
		},
	}
	var response factoryapi.ModelInvocationResponse
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName)) + "/invocations"
	if err := doModelsPOST(cfg.Server, path, request, &response, requestDiagnostics{
		Enabled:      cfg.Verbose,
		Output:       cfg.Diagnostics,
		Command:      "models invoke",
		Server:       cfg.Server,
		ModelName:    strings.TrimSpace(cfg.ModelName),
		Operation:    cfg.Operation,
		RequestBytes: len(cfg.Text),
		SummaryFunc:  func() string { return fmt.Sprintf("worker=%s contentParts=%d", response.Worker, len(response.Content)) },
	}); err != nil {
		return factoryapi.ModelInvocationResponse{}, err
	}
	return response, nil
}

// invokeModelAudio streams non-JSON audio on HTTP 200. It intentionally avoids
// clihttp.PostJSON because success bodies are raw bytes, not JSON payloads.
func invokeModelAudio(cfg invokeOptions) error {
	mode := factoryapi.ModelInvocationResponseMode("AUDIO_STREAM")
	request := factoryapi.ModelInvocationRequest{
		Operation: cfg.Operation,
		Content: &factoryapi.WorkContent{
			mustGeneratedTextContentPart(cfg.Text),
		},
		Options: &factoryapi.ModelInvocationOptions{
			ResponseMode: &mode,
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal invocation request: %w", err)
	}
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName)) + "/invocations"
	endpoint, err := modelsEndpoint(cfg.Server, path)
	if err != nil {
		return err
	}
	logModelsRequest(requestDiagnostics{
		Enabled:      cfg.Verbose,
		Output:       cfg.Diagnostics,
		Command:      "models invoke",
		Server:       cfg.Server,
		ModelName:    strings.TrimSpace(cfg.ModelName),
		Operation:    cfg.Operation,
		OutputPath:   cfg.OutputPath,
		RequestBytes: len(body),
	}, endpoint)
	httpReq, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build invoke request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: modelsRequestTimeout}
	started := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		logModelsResponse(requestDiagnostics{Enabled: cfg.Verbose, Output: cfg.Diagnostics, Command: "models invoke"}, endpoint, 0, time.Since(started), "error=unreachable")
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read invocation error response: %w", readErr)
		}
		logModelsResponse(requestDiagnostics{Enabled: cfg.Verbose, Output: cfg.Diagnostics, Command: "models invoke"}, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("responseBytes=%d", len(responseBody)))
		return modelsRequestError(resp.StatusCode, responseBody)
	}
	output, err := os.Create(cfg.OutputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()
	if _, err := io.Copy(output, resp.Body); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	logModelsResponse(requestDiagnostics{Enabled: cfg.Verbose, Output: cfg.Diagnostics, Command: "models invoke"}, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("outputPath=%s", cfg.OutputPath))
	return nil
}

type pullOptions struct {
	Server      string
	ModelName   string
	Verbose     bool
	Diagnostics io.Writer
}

func pullModel(cfg pullOptions) (factoryapi.ModelPullResponse, error) {
	var response factoryapi.ModelPullResponse
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName)) + "/pull"
	if err := doModelsPOST(cfg.Server, path, map[string]any{}, &response, requestDiagnostics{
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

func doModelsPOST(server, path string, payload any, out any, diagnostics requestDiagnostics) error {
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

	client := &http.Client{Timeout: modelsRequestTimeout}
	started := time.Now()
	resp, err := clihttp.PostJSON(
		context.Background(),
		client,
		endpoint.String(),
		bytes.NewReader(body),
		out,
		clihttp.RequestOptions{
			Diagnostics:  diagnostics.Output,
			Verbose:      diagnostics.Enabled,
			EndpointPath: endpoint.Path,
			LogLabel:     diagnostics.Command,
		},
	)
	if err != nil {
		logModelsResponse(diagnostics, endpoint, 0, time.Since(started), "error=unreachable")
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read models response: %w", readErr)
		}
		logModelsResponse(diagnostics, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("responseBytes=%d", len(responseBody)))
		if pullErr := managedRuntimePullResponseError(resp.StatusCode, responseBody); pullErr != nil {
			if out != nil {
				if err := json.Unmarshal(responseBody, out); err != nil {
					return fmt.Errorf("decode managed runtime pull response: %w", err)
				}
			}
			return pullErr
		}
		return modelsRequestError(resp.StatusCode, responseBody)
	}
	responseBytes, err := modelsResponseBytes(out)
	if err != nil {
		return err
	}
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("responseBytes=%d %s", responseBytes, diagnostics.summary()))
	return nil
}

func doModelsGET(endpoint url.URL, out any, diagnostics requestDiagnostics) error {
	logModelsRequest(diagnostics, endpoint)
	client := &http.Client{Timeout: modelsRequestTimeout}
	started := time.Now()
	resp, err := clihttp.GetJSON(
		context.Background(),
		client,
		endpoint.String(),
		out,
		clihttp.RequestOptions{
			Diagnostics:  diagnostics.Output,
			Verbose:      diagnostics.Enabled,
			EndpointPath: endpoint.Path,
			LogLabel:     diagnostics.Command,
		},
	)
	if err != nil {
		logModelsResponse(diagnostics, endpoint, 0, time.Since(started), "error=unreachable")
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read models response: %w", err)
		}
		logModelsResponse(diagnostics, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("responseBytes=%d", len(body)))
		return modelsRequestError(resp.StatusCode, body)
	}
	responseBytes, err := modelsResponseBytes(out)
	if err != nil {
		return err
	}
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, time.Since(started), fmt.Sprintf("responseBytes=%d %s", responseBytes, diagnostics.summary()))
	return nil
}

func modelsResponseBytes(out any) (int, error) {
	body, err := json.Marshal(out)
	if err != nil {
		return 0, fmt.Errorf("marshal models response: %w", err)
	}
	return len(body), nil
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

func RenderList(response factoryapi.ListModelsResponse, output io.Writer) error {
	if _, err := fmt.Fprintln(output, "NAME\tREADINESS\tLIFECYCLE\tLOCALITY\tOPERATIONS\tMODALITIES\tRESOURCES"); err != nil {
		return err
	}
	results := append([]factoryapi.ModelSummary(nil), response.Results...)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})
	for _, model := range results {
		if _, err := fmt.Fprintf(
			output,
			"%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
			model.Name,
			managedRuntimeReadiness(model.ManagedRuntime),
			managedRuntimeLifecycle(model.ManagedRuntime),
			model.ProviderLocality,
			modelOperationNames(model.Operations),
			modelModalities(model.Modalities),
			len(model.Resources),
		); err != nil {
			return err
		}
	}
	return nil
}

func RenderPull(response factoryapi.ModelPullResponse, output io.Writer) error {
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

func RenderModel(model factoryapi.ModelDetail, output io.Writer) error {
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

func modelOperationNames(operations []factoryapi.ModelOperation) string {
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func modelModalities(modalities []factoryapi.ModelOperationContentType) string {
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
		return fmt.Errorf(
			"managed runtime pull failed (%s readiness %s)",
			response.ManagedRuntimePull.PullOutcome,
			response.ManagedRuntimePull.ReadinessState,
		)
	default:
		return nil
	}
}

func modelsRequestError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		if statusCode == http.StatusNotFound && errResp.Code == factoryapi.ErrorResponseCodeNOTFOUND {
			return fmt.Errorf("%w: %s", ErrModelNotFound, errResp.Message)
		}
		return fmt.Errorf("models request failed (%d): %s", statusCode, errResp.Message)
	}
	preview := strings.TrimSpace(string(body))
	if len(preview) > modelsErrorBodyPreviewSize {
		preview = preview[:modelsErrorBodyPreviewSize] + "..."
	}
	if preview == "" {
		return fmt.Errorf("models request failed (%d)", statusCode)
	}
	return fmt.Errorf("models request failed (%d): %s", statusCode, preview)
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
	_ = part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	})
	return part
}
