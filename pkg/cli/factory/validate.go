package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/factory/validationentry"
)

// ValidateConfig holds parameters for factory validation output.
type ValidateConfig struct {
	Path   string
	JSON   bool
	Output io.Writer
}

// Validate resolves a factory config path and validates it through the shared
// validate-only API contract used by POST /factory-validations.
func Validate(cfg ValidateConfig) error {
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.Path == "" {
		return fmt.Errorf("factory path is required")
	}

	factory, err := loadFactoryAPIFromPath(cfg.Path)
	if err != nil {
		return err
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		return fmt.Errorf("validate factory config: %w", err)
	}

	apiResult := result.FactoryValidationResult()
	if cfg.JSON {
		payload := struct {
			Valid    bool                                   `json:"valid"`
			Targets  []factoryapi.FactoryValidationTarget   `json:"targets"`
			Taxonomy []apisurface.FactoryRuntimeTaxonomyEntry `json:"taxonomy"`
		}{
			Valid:    len(apiResult.Targets) == 0,
			Targets:  apiResult.Targets,
			Taxonomy: apisurface.FactoryRuntimeTaxonomySummary(factory),
		}
		if err := json.NewEncoder(cfg.Output).Encode(payload); err != nil {
			return err
		}
		if !payload.Valid {
			return fmt.Errorf("factory validation found blocking issues")
		}
		return nil
	}

	return apisurface.RenderFactoryValidationHuman(factory, apiResult, cfg.Output)
}

func loadFactoryAPIFromPath(path string) (factoryapi.Factory, error) {
	canonical, err := factoryconfig.FlattenFactoryConfig(path)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	var factory factoryapi.Factory
	if err := json.Unmarshal(canonical, &factory); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("parse factory config: %w", err)
	}
	return factory, nil
}
