// Package wire constructs the Factory Definitions authoring_layout subservice
// from exact injected layout-parse, transform, and durable-write ports.
package wire

import (
	"fmt"

	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/contracts"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

// NewService constructs the private authoring_layout subservice from exact
// injected authoring ports. It does not choose host filesystem adapters or
// take Wire/root construction ownership.
func NewService(
	validator authoringlayoutcontracts.LayoutValidator,
	validateDefinition authoringlayoutcontracts.DefinitionValidationOperation,
	mapInput authoringlayoutcontracts.LayoutPayloadMapper,
	decodeFactory authoringlayoutcontracts.FactoryConfigJSONDecoder,
	normalizeAuthored authoringlayoutcontracts.AuthoredFactoryNormalizer,
	encodeFactory authoringlayoutcontracts.FactoryConfigJSONEncoder,
	write authoringlayoutcontracts.LayoutWriter,
	validate authoringlayoutcontracts.LayoutValidatorFunc,
	flatten authoringlayoutcontracts.LayoutFlattener,
	expand authoringlayoutcontracts.LayoutExpander,
	fileSystem authoringlayoutcontracts.PersistenceFileSystem,
	requireDefinitionDir authoringlayoutcontracts.DefinitionDirectoryRequirer,
	directories authoringlayoutcontracts.DirectoryReplacementStore,
) (authoringlayout.Service, error) {
	if validator == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: validator is required")
	}
	if validateDefinition == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: definition validation operation is required")
	}
	if mapInput == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: payload mapper is required")
	}
	if decodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory decoder is required")
	}
	if normalizeAuthored == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: authored normalizer is required")
	}
	if encodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory encoder is required")
	}
	if write == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout writer is required")
	}
	if validate == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout validator is required")
	}
	if flatten == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout flattener is required")
	}
	if expand == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout expander is required")
	}
	if fileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: persistence filesystem is required")
	}
	if requireDefinitionDir == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: definition directory validator is required")
	}
	if directories == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: directory replacement store is required")
	}
	service := authoringlayoutservice.New(
		validator,
		validateDefinition,
		mapInput,
		decodeFactory,
		normalizeAuthored,
		encodeFactory,
		write,
		validate,
		flatten,
		expand,
		fileSystem,
		requireDefinitionDir,
		directories,
	)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: implementation rejected its dependencies")
	}
	return service, nil
}
