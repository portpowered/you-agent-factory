package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factoryconfig "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

// ValidateConfig holds parameters for factory validation output.
type ValidateConfig struct {
	Context context.Context
	Path    string
	JSON    bool
	Output  io.Writer
}

// ValidateWithServices validates through injected Factory Definitions root
// capabilities. The transport decodes supplied representation bytes but owns
// no filesystem or path-resolution behavior.
func ValidateWithServices(
	cfg ValidateConfig,
	validate factorydefinitions.SubmittedDefinitionValidationOperation,
	loadSource factorydefinitions.AuthoredFactorySourceLoader,
) error {
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Path == "" {
		return fmt.Errorf("factory path is required")
	}
	if loadSource == nil {
		return fmt.Errorf("authored Factory Definition source loader is required")
	}

	source, err := loadSource(cfg.Path)
	if err != nil {
		return err
	}
	factory, err := factoryconfig.DecodeAuthoredFactoryAPI(source)
	if err != nil {
		return err
	}

	result, err := validationentry.ValidateFactoryAPI(
		cfg.Context,
		factory,
		validate,
	)
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
