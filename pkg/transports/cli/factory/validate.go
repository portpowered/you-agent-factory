package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
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

	factory, err := factoryconfig.LoadAuthoredFactoryAPIFromPath(cfg.Path)
	if err != nil {
		return err
	}

	result, err := validationentry.ValidateFactoryAPI(context.Background(), factory, factoryvalidation.Options{
		Profile: factoryvalidation.ProfileTopology,
	})
	if err != nil {
		return fmt.Errorf("validate factory config: %w", err)
	}

	apiResult := apisurface.FactoryValidationResultToAPI(result)
	if cfg.JSON {
		payload := struct {
			Valid    bool                                     `json:"valid"`
			Targets  []factoryapi.FactoryValidationTarget     `json:"targets"`
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
