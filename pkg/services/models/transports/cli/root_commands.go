package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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
			return mapModelsRootError(err)
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
	if service.http == nil {
		return fmt.Errorf("CLI HTTP protocol is required for remote models pull")
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	response, err := pullModel(pullOptions{
		Context:     cfg.Context,
		Server:      cfg.Server,
		ModelName:   modelName,
		Verbose:     cfg.Verbose,
		Diagnostics: cfg.Diagnostics,
		HTTP:        service.http,
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
