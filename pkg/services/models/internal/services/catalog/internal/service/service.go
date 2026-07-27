package service

import (
	"context"
	"errors"
	"fmt"
	"sort"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

type service struct {
	scopes runtimescopes.Service
}

var _ catalog.Service = (*service)(nil)

// New constructs an inert catalog over the Models-owned Runtime Scopes
// authority.
func New(scopes runtimescopes.Service) catalog.Service {
	return &service{scopes: scopes}
}

func (s *service) ListCatalog(
	ctx context.Context,
	request models.ListModelsRequest,
) (models.ListModelsResult, error) {
	if err := ctx.Err(); err != nil {
		return models.ListModelsResult{}, err
	}
	if request.Scope.IsZero() {
		return models.ListModelsResult{}, models.ErrRuntimeScopeInvalid
	}
	if s == nil || s.scopes == nil {
		return models.ListModelsResult{}, models.ErrUnavailable
	}

	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.ListModelsResult{}, catalogScopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return models.ListModelsResult{}, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if runtimeConfig == nil {
		return models.ListModelsResult{}, models.ErrUnavailable
	}

	entries := localmodels.BuildCatalogWithRuntime(
		runtimeConfig,
		nil,
		localmodels.DefaultManagedRuntimeSourceResolver(),
	)
	result := models.ListModelsResult{Models: make([]models.Summary, 0, len(entries))}
	for _, entry := range entries {
		result.Models = append(result.Models, stableSummary(entry.Summary))
	}
	sort.Slice(result.Models, func(i, j int) bool {
		return localmodels.CanonicalModelName(result.Models[i].Name) <
			localmodels.CanonicalModelName(result.Models[j].Name)
	})
	return result, nil
}

func catalogScopeError(err error) error {
	switch {
	case errors.Is(err, runtimescopes.ErrScopeForeign):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeForeign, err)
	case errors.Is(err, runtimescopes.ErrScopeUnknown):
		return fmt.Errorf("%w: %v", models.ErrRuntimeScopeStale, err)
	default:
		return models.ErrUnavailable
	}
}

func stableSummary(summary models.Summary) models.Summary {
	summary = summary.Clone()
	sortOperations(summary.Operations)
	sort.Strings(summary.Modalities)
	sort.Slice(summary.Resources, func(i, j int) bool {
		return summary.Resources[i].Name < summary.Resources[j].Name
	})
	sortOperations(summary.ManagedRuntime.SupportedOperations)
	return summary
}

func sortOperations(operations []models.Operation) {
	for i := range operations {
		sortOperationSlots(operations[i].Inputs)
		sortOperationSlots(operations[i].Outputs)
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Name < operations[j].Name
	})
}

func sortOperationSlots(slots []models.OperationSlot) {
	for i := range slots {
		sort.Strings(slots[i].ContentTypes)
	}
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].Name < slots[j].Name
	})
}
