// Package models defines model-discovery command behavior.
package models

import (
	"bytes"
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
)

const (
	modelsRequestTimeout       = 10 * time.Second
	modelsErrorBodyPreviewSize = 200
)

var ErrModelNotFound = errors.New("model not found")

type ListConfig struct {
	Port   int
	JSON   bool
	Output io.Writer
}

type InspectConfig struct {
	ModelName string
	Port      int
	JSON      bool
	Output    io.Writer
}

type InvokeConfig struct {
	ModelName  string
	Operation  string
	Text       string
	OutputPath string
	Port       int
	JSON       bool
	Output     io.Writer
}

type PullConfig struct {
	ModelName string
	Port      int
	JSON      bool
	Output    io.Writer
}

func List(cfg ListConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	response, err := QueryList(cfg.Port)
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
	model, err := QueryModel(cfg.Port, cfg.ModelName)
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
		response, err := invokeModelMetadata(cfg.Port, modelName, operation, text)
		if err != nil {
			return err
		}
		return json.NewEncoder(cfg.Output).Encode(response)
	}

	outputPath := strings.TrimSpace(cfg.OutputPath)
	if outputPath == "" {
		return fmt.Errorf("--output is required unless --json is set")
	}
	if err := invokeModelAudio(cfg.Port, modelName, operation, text, outputPath); err != nil {
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
	response, err := pullModel(cfg.Port, modelName)
	if err != nil {
		return err
	}
	if cfg.JSON {
		return json.NewEncoder(cfg.Output).Encode(response)
	}
	return RenderPull(response, cfg.Output)
}

func QueryList(port int) (factoryapi.ListModelsResponse, error) {
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
		Path:   "/models",
	}
	var response factoryapi.ListModelsResponse
	if err := doModelsGET(endpoint, &response); err != nil {
		return factoryapi.ListModelsResponse{}, err
	}
	return response, nil
}

func QueryModel(port int, modelName string) (factoryapi.ModelDetail, error) {
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
		Path:   "/models/" + url.PathEscape(strings.TrimSpace(modelName)),
	}
	var response factoryapi.ModelDetail
	if err := doModelsGET(endpoint, &response); err != nil {
		return factoryapi.ModelDetail{}, err
	}
	return response, nil
}

func invokeModelMetadata(port int, modelName, operation, text string) (factoryapi.ModelInvocationResponse, error) {
	request := factoryapi.ModelInvocationRequest{
		Operation: operation,
		Content: &factoryapi.WorkContent{
			mustGeneratedTextContentPart(text),
		},
	}
	var response factoryapi.ModelInvocationResponse
	if err := doModelsPOST(port, "/models/"+url.PathEscape(strings.TrimSpace(modelName))+"/invocations", request, &response); err != nil {
		return factoryapi.ModelInvocationResponse{}, err
	}
	return response, nil
}

func invokeModelAudio(port int, modelName, operation, text, outputPath string) error {
	mode := factoryapi.ModelInvocationResponseMode("AUDIO_STREAM")
	request := factoryapi.ModelInvocationRequest{
		Operation: operation,
		Content: &factoryapi.WorkContent{
			mustGeneratedTextContentPart(text),
		},
		Options: &factoryapi.ModelInvocationOptions{
			ResponseMode: &mode,
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal invocation request: %w", err)
	}
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
		Path:   "/models/" + url.PathEscape(strings.TrimSpace(modelName)) + "/invocations",
	}
	httpReq, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build invoke request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: modelsRequestTimeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read invocation error response: %w", readErr)
		}
		return modelsRequestError(resp.StatusCode, responseBody)
	}
	output, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()
	if _, err := io.Copy(output, resp.Body); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

func pullModel(port int, modelName string) (factoryapi.ModelPullResponse, error) {
	var response factoryapi.ModelPullResponse
	if err := doModelsPOST(port, "/models/"+url.PathEscape(strings.TrimSpace(modelName))+"/pull", map[string]any{}, &response); err != nil {
		return factoryapi.ModelPullResponse{}, err
	}
	return response, nil
}

func doModelsPOST(port int, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal models request: %w", err)
	}
	endpoint := url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("localhost:%d", port),
		Path:   path,
	}
	client := &http.Client{Timeout: modelsRequestTimeout}
	resp, err := client.Post(endpoint.String(), "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return modelsRequestError(resp.StatusCode, responseBody)
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("parse models response: %w", err)
	}
	return nil
}

func doModelsGET(endpoint url.URL, out any) error {
	client := &http.Client{Timeout: modelsRequestTimeout}
	resp, err := client.Get(endpoint.String())
	if err != nil {
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return modelsRequestError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parse models response: %w", err)
	}
	return nil
}

func RenderList(response factoryapi.ListModelsResponse, output io.Writer) error {
	if _, err := fmt.Fprintln(output, "NAME\tLOCALITY\tSTATUS\tLOAD STATE\tOPERATIONS\tMODALITIES\tRESOURCES"); err != nil {
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
			model.ProviderLocality,
			model.Status,
			model.LoadState,
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
	if _, err := fmt.Fprintf(output, "MODEL\tOUTCOME\tREVISION\tCACHE PATH\n%s\t%s\t%s\t%s\n", response.ModelName, response.Outcome, response.Revision, response.CachePath); err != nil {
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
		{label: "Locality", value: string(model.ProviderLocality)},
		{label: "Status", value: string(model.Status)},
		{label: "Load State", value: string(model.LoadState)},
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
	if len(model.Diagnostics) > 0 {
		keys := make([]string, 0, len(model.Diagnostics))
		for key := range model.Diagnostics {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(output, "Diagnostics:"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(output, "- %s=%s\n", key, model.Diagnostics[key]); err != nil {
				return err
			}
		}
	}
	return nil
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

func modelsRequestError(statusCode int, body []byte) error {
	var errResp factoryapi.ErrorResponse
	if json.Unmarshal(body, &errResp) == nil && errResp.Message != "" {
		if statusCode == http.StatusNotFound && errResp.Code == factoryapi.NOTFOUND {
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

func mustGeneratedTextContentPart(text string) factoryapi.WorkContentPart {
	var part factoryapi.WorkContentPart
	_ = part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeTextUpper,
		Text: text,
	})
	return part
}
