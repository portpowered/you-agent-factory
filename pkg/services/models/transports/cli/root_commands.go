package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func (service *rootService) List(cfg ListConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(cfg.Server) != "" {
		return service.listRemote(cfg)
	}
	return service.withCatalogScope(cfg.Context, func(scope modelinference.RuntimeScopeRef) error {
		listed, err := service.models.ListCatalog(cfg.Context, modelinference.ListModelsRequest{Scope: scope})
		if err != nil {
			return mapModelsRootError(err)
		}
		response := listToGenerated(modelinference.List{Results: listed.Models})
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		return renderList(response, cfg.Output)
	})
}

func (service *rootService) Inspect(cfg InspectConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if strings.TrimSpace(cfg.Server) != "" {
		return service.inspectRemote(cfg)
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	return service.withCatalogScope(cfg.Context, func(scope modelinference.RuntimeScopeRef) error {
		result, err := service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
			Scope: scope, Name: modelName,
		})
		if err != nil {
			return mapModelsClientError(err)
		}
		response := detailToGenerated(result.Model)
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		return renderModel(response, cfg.Output)
	})
}

func (service *rootService) Pull(cfg PullConfig) error {
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
	if strings.TrimSpace(cfg.Server) != "" {
		return service.pullRemote(cfg)
	}
	return service.withCatalogScope(cfg.Context, func(scope modelinference.RuntimeScopeRef) error {
		result, err := service.models.PullModelForScope(cfg.Context, modelinference.PullModelRequest{
			Scope: scope, Name: modelName,
		})
		response := pullResultToGenerated(result)
		if cfg.JSON {
			if encodeErr := json.NewEncoder(cfg.Output).Encode(response); encodeErr != nil {
				return encodeErr
			}
		}
		if err != nil {
			return mapModelsRootError(err)
		}
		if cfg.JSON {
			return nil
		}
		return renderPull(response, cfg.Output)
	})
}

func (service *rootService) Remove(cfg RemoveConfig) error {
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
	if strings.TrimSpace(cfg.Server) != "" {
		return service.removeRemote(cfg)
	}
	return service.withCatalogScope(cfg.Context, func(scope modelinference.RuntimeScopeRef) error {
		result, err := service.models.RemoveModelAssets(cfg.Context, modelinference.RemoveModelAssetsRequest{
			Scope: scope,
			Name:  modelName,
		})
		if err != nil {
			return mapModelsRootError(err)
		}
		response := removeResultToGenerated(result)
		if cfg.JSON {
			return json.NewEncoder(cfg.Output).Encode(response)
		}
		return renderRemove(response, cfg.Output)
	})
}

func (service *httpService) validateModelInvoke(cfg InvokeConfig, modelName, operation string) error {
	// An explicit server selects the HTTP fallback even when the process also
	// carries a locally composed Models root. The composition facade routes
	// server-bound invokes here, so validation must describe the target the
	// caller selected rather than silently opening the local Factory.
	if strings.TrimSpace(cfg.Server) != "" {
		if service.http == nil {
			return fmt.Errorf("CLI HTTP protocol is required for remote models invoke validation")
		}
		model, err := queryModel(queryOptions{
			Context: cfg.Context, Server: cfg.Server, ModelName: modelName,
			Verbose: cfg.Verbose, Diagnostics: cfg.Diagnostics, HTTP: service.http,
		})
		if err != nil {
			return err
		}
		if !generatedModelSupportsOperation(model, operation) {
			return fmt.Errorf("model %q does not support operation %q", modelName, operation)
		}
		return nil
	}
	if service.models != nil && (service.openInvokeScope != nil || service.openCatalogScope != nil) {
		var scope InvokeRuntimeScope
		var err error
		if service.openInvokeScope != nil {
			scope, err = service.openInvokeScope(cfg.Context, cfg)
		} else {
			scope, err = service.openCatalogScope(cfg.Context)
		}
		if err != nil {
			return mapModelsRootError(err)
		}
		if scope.Close != nil {
			defer func() { _ = scope.Close(cfg.Context) }()
		}
		_, err = service.models.GetCatalogModel(cfg.Context, modelinference.GetModelRequest{
			Scope: scope.Scope, Name: modelName, Operation: operation,
		})
		if err != nil {
			return mapModelsRootError(err)
		}
		return nil
	}
	// Older embedded callers may not provide either a Models root or an HTTP
	// target. Preserve their validation-only compatibility envelope because
	// there is no catalog against which this transport can validate.
	return nil
}

func generatedModelSupportsOperation(model factoryapi.ModelDetail, operation string) bool {
	for _, candidate := range model.Operations {
		if candidate.Name == operation {
			return true
		}
	}
	for _, capability := range model.Capabilities {
		for _, candidate := range capability.Operations {
			if candidate.Name == operation {
				return true
			}
		}
	}
	for _, candidate := range model.ManagedRuntime.SupportedOperations {
		if candidate.Name == operation {
			return true
		}
	}
	return false
}

func (service *rootService) withCatalogScope(
	ctx context.Context,
	run func(modelinference.RuntimeScopeRef) error,
) error {
	if service.openCatalogScope == nil {
		return fmt.Errorf("models catalog scope opener is required")
	}
	scope, err := service.openCatalogScope(ctx)
	if err != nil {
		return mapModelsRootError(err)
	}
	if scope.Close != nil {
		defer func() {
			_ = scope.Close(ctx)
		}()
	}
	return run(scope.Scope)
}

func (service *rootService) listRemote(cfg ListConfig) error {
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models list")
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

func (service *rootService) inspectRemote(cfg InspectConfig) error {
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models inspect")
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

func (service *rootService) pullRemote(cfg PullConfig) error {
	pullHTTP := service.pullHTTP
	if pullHTTP == nil {
		pullHTTP = service.http
	}
	if pullHTTP == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models pull")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	response, err := pullModel(pullOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		ModelName:   modelName,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        pullHTTP,
		Now:         service.now,
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

func (service *rootService) removeRemote(cfg RemoveConfig) error {
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models remove")
	}
	response, err := removeModel(removeOptions{
		Context: cfg.Context, Server: cfg.Server, ModelName: cfg.ModelName,
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
		return modelsRequestError(resp.StatusCode, body, resp)
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

func joinedCLIInputSlots(operation modelinference.Operation) (map[string]modelinference.OperationSlot, []string) {
	slots := make(map[string]modelinference.OperationSlot, len(operation.Inputs))
	validNames := make([]string, 0, len(operation.Inputs))
	for _, slot := range operation.Inputs {
		name := strings.TrimSpace(slot.Name)
		if name == "" {
			continue
		}
		slots[strings.ToLower(name)] = slot
		validNames = append(validNames, name)
	}
	sort.Strings(validNames)
	return slots, validNames
}

func validateJoinedCLIInputMappings(
	mappings []joinedCLIInputBinding,
	slots map[string]modelinference.OperationSlot,
	modelName string,
	operation string,
	validNames []string,
) (map[string]int, error) {
	counts := make(map[string]int, len(mappings))
	for _, mapping := range mappings {
		key := strings.ToLower(mapping.slot)
		slot, exists := slots[key]
		if !exists {
			return nil, joinedCLIInputFailure(
				modelName, operation, modelinference.InvocationFailureClassInvalidSlot,
				fmt.Sprintf("unknown input slot %q; valid slots: %s", mapping.slot, strings.Join(validNames, ", ")),
				mapping.slot, validNames,
			)
		}
		counts[key]++
		if !slot.Repeatable && counts[key] > 1 {
			return nil, joinedCLIInputFailure(
				modelName, operation, modelinference.InvocationFailureClassSlotArity,
				fmt.Sprintf("input slot %q accepts at most one value", slot.Name),
				slot.Name, []string{"1"},
			)
		}
	}
	return counts, nil
}

func validateJoinedCLIRequiredSlots(
	operation modelinference.Operation,
	counts map[string]int,
	modelName string,
	operationName string,
	validNames []string,
) error {
	missing := make([]string, 0)
	for _, slot := range operation.Inputs {
		name := strings.TrimSpace(slot.Name)
		if slot.Required != nil && *slot.Required && counts[strings.ToLower(name)] == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return joinedCLIInputFailure(
		modelName, operationName, modelinference.InvocationFailureClassInvalidSlot,
		"required input slot is missing: "+strings.Join(missing, ", "), missing[0], validNames,
	)
}

func (service *rootService) readJoinedCLIInputs(
	cfg InvokeConfig,
	modelName string,
	operation string,
	mappings []joinedCLIInputBinding,
	slots map[string]modelinference.OperationSlot,
) ([]modelinference.InferenceInput, error) {
	inputs := make([]modelinference.InferenceInput, 0, len(mappings))
	for _, mapping := range mappings {
		if err := cfg.Context.Err(); err != nil {
			return nil, err
		}
		slot := slots[strings.ToLower(mapping.slot)]
		input, err := service.joinedCLIInput(cfg, modelName, operation, mapping, slot)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}
