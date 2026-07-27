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
	runtimeConfig, err := s.resolveRuntimeConfig(ctx, request.Scope)
	if err != nil {
		return models.ListModelsResult{}, err
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

func (s *service) GetCatalogModel(
	ctx context.Context,
	request models.GetModelRequest,
) (models.GetModelResult, error) {
	runtimeConfig, err := s.resolveRuntimeConfig(ctx, request.Scope)
	if err != nil {
		return models.GetModelResult{}, err
	}
	if err := request.Validate(); err != nil {
		return models.GetModelResult{}, err
	}

	entries := localmodels.BuildCatalogWithRuntime(
		runtimeConfig,
		nil,
		localmodels.DefaultManagedRuntimeSourceResolver(),
	)
	entry, ok := entries[localmodels.CanonicalModelName(request.Name)]
	if !ok {
		return models.GetModelResult{}, fmt.Errorf("%w: %s", models.ErrNotFound, request.Name)
	}
	detail := stableDetail(entry.Detail)
	if operation := request.Operation; operation != "" && !supportsOperation(detail, operation) {
		return models.GetModelResult{}, fmt.Errorf(
			"%w: model %q does not support operation %q",
			models.ErrUnsupportedOperation,
			detail.Name,
			operation,
		)
	}
	return models.GetModelResult{Model: detail}, nil
}

func (s *service) resolveRuntimeConfig(
	ctx context.Context,
	scope models.RuntimeScopeRef,
) (*models.RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope.IsZero() {
		return nil, models.ErrRuntimeScopeInvalid
	}
	if s == nil || s.scopes == nil {
		return nil, models.ErrUnavailable
	}

	binding, err := s.scopes.Resolve(runtimescopes.Reference(scope.String()))
	if err != nil {
		return nil, catalogScopeError(err)
	}
	if binding.RuntimeConfig == nil {
		return nil, models.ErrUnavailable
	}
	runtimeConfig := binding.RuntimeConfig()
	if runtimeConfig == nil {
		return nil, models.ErrUnavailable
	}
	return runtimeConfig, nil
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

func stableDetail(detail models.Detail) models.Detail {
	detail = detail.Clone()
	detail.Summary = stableSummary(detail.Summary)
	if len(detail.Sources) == 0 {
		detail.Sources = sourceMetadata(detail.ManagedRuntime.Diagnostics)
	}
	for i := range detail.Capabilities {
		sortOperations(detail.Capabilities[i].Operations)
		sort.Strings(detail.Capabilities[i].ResourceNames)
	}
	sort.Slice(detail.Capabilities, func(i, j int) bool {
		return detail.Capabilities[i].Worker < detail.Capabilities[j].Worker
	})
	sort.Slice(detail.Sources, func(i, j int) bool {
		left, right := detail.Sources[i], detail.Sources[j]
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.Reference != right.Reference {
			return left.Reference < right.Reference
		}
		return left.Revision < right.Revision
	})
	return detail
}

func sourceMetadata(diagnostics map[string]string) []models.SourceMetadata {
	provider := diagnostics["sourceKind"]
	reference := diagnostics["sourceId"]
	revision := diagnostics["revision"]
	if provider == "" && reference == "" && revision == "" {
		return nil
	}
	return []models.SourceMetadata{{
		Provider:  provider,
		Reference: reference,
		Revision:  revision,
	}}
}

func supportsOperation(detail models.Detail, requested string) bool {
	for _, operation := range detail.Operations {
		if operation.Name == requested {
			return true
		}
	}
	for _, capability := range detail.Capabilities {
		for _, operation := range capability.Operations {
			if operation.Name == requested {
				return true
			}
		}
	}
	return false
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
