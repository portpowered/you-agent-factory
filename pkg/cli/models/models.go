// Package models defines model-discovery command behavior.
package models

import (
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
