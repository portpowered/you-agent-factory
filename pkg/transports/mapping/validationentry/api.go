// Package validationentry maps and validates generated Factory payloads
// through the canonical Factory validation owner.
package validationentry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

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
