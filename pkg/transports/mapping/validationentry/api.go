// Package validationentry maps and validates generated Factory payloads
// through the canonical Factory validation owner.
package validationentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// ValidateEditableFactorySnapshot retains the transport-level compatibility
// entrypoint used by callers that only need detached layout and topology
// validation. Application composition uses MapEditableFactorySnapshot with the
// Factory Definitions-owned validation operation.
func ValidateEditableFactorySnapshot(
	snapshot *interfaces.FactorySnapshot,
	_ interfaces.WorkstationLoader,
) error {
	if snapshot == nil {
		return fmt.Errorf("%w: Factory snapshot is required", interfaces.ErrInvalidNamedFactory)
	}
	payload := append([]byte(nil), (*snapshot)...)
	if err := factorymapping.ValidatePortableLayoutBoundaryJSON(payload); err != nil {
		if target, ok := factorymapping.PortableLayoutValidationTarget(err); ok {
			return apisurface.NewTopologyValidationError(
				"Factory layout contains an invalid authored value.",
				apisurface.FactoryValidationTargetsToAPI([]interfaces.ValidationTarget{target}),
			)
		}
		return fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	var submitted factoryapi.Factory
	if err := snapshot.Decode(&submitted); err != nil {
		return fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(submitted)
	if err != nil {
		return fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	result := factoryvalidation.New(nil).Validate(context.Background(), &cfg, nil)
	if !result.HasBlockingTargets() {
		return nil
	}
	return apisurface.NewTopologyValidationError(
		"Factory topology contains invalid graph references.",
		apisurface.FactoryValidationTargetsToAPI(result.BlockingTargets()),
	)
}

// MapFactoryJSONForPersistence maps one serialized public Factory definition
// into a detached pre-persist request. It does not invoke validation or
// interpret findings.
func MapFactoryJSONForPersistence(
	payload []byte,
	loadCanonicalFactory interfaces.CanonicalFactoryJSONLoader,
) (interfaces.DefinitionValidationRequest, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf(
			"%w: parse factory config: %w",
			interfaces.ErrInvalidNamedFactory,
			err,
		)
	}
	request, err := prePersistFactoryValidationRequest(
		factory,
		nil,
		nil,
		loadCanonicalFactory,
	)
	if err != nil {
		if errors.Is(err, interfaces.ErrInvalidNamedFactory) {
			return interfaces.DefinitionValidationRequest{}, err
		}
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	return request, nil
}

// MapEditableFactorySnapshot maps a detached Factory-owned definition into a
// pre-persist request. It does not invoke validation or interpret findings.
func MapEditableFactorySnapshot(
	snapshot *interfaces.FactorySnapshot,
	workstationLoader interfaces.WorkstationLoader,
	loadCanonicalFactory interfaces.CanonicalFactoryJSONLoader,
) (interfaces.DefinitionValidationRequest, error) {
	if snapshot == nil {
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf("%w: Factory snapshot is required", interfaces.ErrInvalidNamedFactory)
	}
	var submitted factoryapi.Factory
	if err := snapshot.Decode(&submitted); err != nil {
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	request, err := prePersistFactoryValidationRequest(
		submitted,
		workstationLoader,
		nil,
		loadCanonicalFactory,
	)
	if err != nil {
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf("%w: %v", interfaces.ErrInvalidNamedFactory, err)
	}
	return request, nil
}

// ValidateFactoryAPI maps one public validation request and invokes the exact
// Factory Definitions validation operation. The operation owns its fixed
// topology profile; HTTP and CLI callers cannot select validation phases.
func ValidateFactoryAPI(
	ctx context.Context,
	factory factoryapi.Factory,
	operation interfaces.SubmittedDefinitionValidationOperation,
) (interfaces.ValidationResult, error) {
	if operation == nil {
		return interfaces.ValidationResult{}, fmt.Errorf("Factory Definition validation operation is required")
	}
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return interfaces.ValidationResult{}, err
	}
	return operation.ValidateSubmittedDefinition(ctx, interfaces.SubmittedDefinitionValidationRequest{
		Config:   &cfg,
		Taxonomy: submittedDefinitionTaxonomyFromAPI(factory),
	})
}

func prePersistFactoryValidationRequest(
	factory factoryapi.Factory,
	workstationLoader interfaces.WorkstationLoader,
	workflowSourceReader interfaces.WorkflowSourceReader,
	loadCanonicalFactory interfaces.CanonicalFactoryJSONLoader,
) (interfaces.DefinitionValidationRequest, error) {
	cfg, err := factorymapping.FactoryConfigFromOpenAPI(factory)
	if err != nil {
		return interfaces.DefinitionValidationRequest{}, err
	}
	request := interfaces.DefinitionValidationRequest{
		Config:                 &cfg,
		WorkstationLoader:      workstationLoader,
		WorkflowSourceReader:   workflowSourceReader,
		CanonicalFactoryLoader: loadCanonicalFactory,
		SubmittedTaxonomy:      submittedDefinitionTaxonomyFromAPI(factory),
	}
	request.CanonicalPayload, err = json.Marshal(factory)
	if err != nil {
		return interfaces.DefinitionValidationRequest{}, fmt.Errorf("marshal factory: %w", err)
	}
	return request, nil
}
