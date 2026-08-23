package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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

type removeOptions struct {
	Context     context.Context
	Server      string
	ModelName   string
	Verbose     bool
	Diagnostics io.Writer
	HTTP        clihttp.Protocol
}

func removeModel(cfg removeOptions) (factoryapi.ModelRemoveResponse, error) {
	path := "/models/" + url.PathEscape(strings.TrimSpace(cfg.ModelName))
	endpoint, err := modelsEndpoint(cfg.Server, path)
	if err != nil {
		return factoryapi.ModelRemoveResponse{}, err
	}
	var response factoryapi.ModelRemoveResponse
	if err := doModelsDELETE(cfg.Context, cfg.HTTP, endpoint, &response, requestDiagnostics{
		Enabled:   cfg.Verbose,
		Output:    cfg.Diagnostics,
		Command:   "models remove",
		Server:    cfg.Server,
		ModelName: strings.TrimSpace(cfg.ModelName),
		SummaryFunc: func() string {
			return fmt.Sprintf(
				"removeOutcome=%s revision=%s bytesRemoved=%d",
				response.Outcome, response.Revision, response.BytesRemoved,
			)
		},
	}); err != nil {
		return factoryapi.ModelRemoveResponse{}, err
	}
	return response, nil
}

func doModelsDELETE(
	ctx context.Context,
	transport clihttp.Protocol,
	endpoint url.URL,
	out any,
	diagnostics requestDiagnostics,
) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}
	if transport == nil {
		return fmt.Errorf("CLI HTTP protocol is required")
	}
	logModelsRequest(diagnostics, endpoint)
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("build models remove request: %w", err)
	}
	response, err := transport.Execute(request)
	if err != nil {
		logModelsResponse(diagnostics, endpoint, 0, response.Duration, "error=unreachable")
		return fmt.Errorf("models endpoint not reachable at %s: %w", endpoint.String(), err)
	}
	resp := response.HTTP
	if resp == nil {
		return fmt.Errorf("models endpoint returned no response")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read models response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d", len(body)))
		return modelsRequestError(resp.StatusCode, body)
	}
	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode models remove response: %w", err)
		}
	}
	logModelsResponse(diagnostics, endpoint, resp.StatusCode, response.Duration, fmt.Sprintf("responseBytes=%d %s", len(body), diagnostics.summary()))
	return nil
}

func renderRemove(response factoryapi.ModelRemoveResponse, output io.Writer) error {
	_, err := fmt.Fprintf(
		output,
		"MODEL\tREMOVE OUTCOME\tREVISION\tCACHE PATH\tBYTES REMOVED\n%s\t%s\t%s\t%s\t%s (%d bytes)\n",
		response.ModelName,
		response.Outcome,
		response.Revision,
		response.CachePath,
		humanByteSize(response.BytesRemoved),
		response.BytesRemoved,
	)
	return err
}

func removeResultToGenerated(result modelinference.RemoveModelAssetsResult) factoryapi.ModelRemoveResponse {
	return factoryapi.ModelRemoveResponse{
		ModelName:    result.ModelName,
		Revision:     result.Revision,
		CachePath:    result.CachePath,
		Outcome:      factoryapi.ModelRemoveOutcome(result.Outcome),
		BytesRemoved: result.BytesRemoved,
	}
}
