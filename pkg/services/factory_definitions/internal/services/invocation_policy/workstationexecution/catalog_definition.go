package workstationexecution

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// ResolveExecutionDefinition returns the fully interpolated Factory
// configuration used by a one-shot runtime opening. It shares the catalog
// resolver's interpolation and validation policy, but retains the complete
// Factory configuration shape needed by Runtime activation rather than
// reducing it to detached catalog entries.
//
// The returned definition is detached from the request. Callers should only
// use this operation after effective invocation arguments have been prepared;
// a nil argument set deliberately leaves the authored definition untouched so
// long-lived runtimes can accept a later invocation.
func ResolveExecutionDefinition(
	ctx context.Context,
	request ResolveExecutionCatalogRequest,
) (*FactoryConfig, error) {
	definition, _, err := ResolveExecutionDefinitionWithProvenance(ctx, request)
	return definition, err
}

// ResolveExecutionDefinitionWithProvenance resolves one effective Factory
// definition and records the exact rendered spans contributed by declared
// sensitive invocation arguments. The provenance is intentionally separate
// from the definition so the resolved value never needs to cross a recording
// boundary.
func ResolveExecutionDefinitionWithProvenance(
	ctx context.Context,
	request ResolveExecutionCatalogRequest,
) (*FactoryConfig, []factorydefinitions.InvocationSensitiveJSONSpan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if request.EffectiveDefinition == nil {
		_, err := invalidExecutionCatalogResult()
		return nil, nil, err
	}
	definition, err := CloneFactoryConfig(request.EffectiveDefinition)
	if err != nil {
		return nil, nil, fmt.Errorf("clone effective Factory definition: %w", err)
	}
	preserveExecutionRuntimeFields(definition, request.EffectiveDefinition)
	if request.Invocation.Arguments == nil {
		return definition, nil, nil
	}

	args := work.CloneInvocationArguments(request.Invocation.Arguments)
	provenance := &invocationInterpolationProvenance{}
	for index := range definition.Workers {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		resolved, err := interpolateExecutionWorker(
			definition.Workers[index], args, request.Invocation.ReadFile,
			invocationInterpolationContext{
				basePointer: fmt.Sprintf("/workers/%d", index),
				provenance:  provenance,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		definition.Workers[index] = resolved
	}
	for index := range definition.Workstations {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		resolved, err := interpolateExecutionWorkstation(
			definition.Workstations[index], args, request.Invocation.ReadFile,
			invocationInterpolationContext{
				basePointer: fmt.Sprintf("/workstations/%d", index),
				provenance:  provenance,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		definition.Workstations[index] = resolved
	}
	// Validate the already-interpolated definition through the same pure catalog
	// policy. This catches malformed identities and cross-reference errors
	// before Runtime or provider selection can observe the definition.
	if _, err := resolveExecutionCatalogDefinition(
		ctx,
		definition,
		nil,
		nil,
		cloneExecutionCatalogReferences(request.References),
	); err != nil {
		return nil, nil, err
	}
	return definition, provenance.values(), nil
}
