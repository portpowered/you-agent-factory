// Package wire constructs the Factory Definitions authoring_layout subservice
// from exact injected layout-parse, transform, and durable-write ports.
package wire

import (
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

// NewService constructs the private authoring_layout owner from direct ports.
// Construction does not choose filesystem adapters or execute layout operations.
func NewService(
	validator factorydefinitions.Validator,
	mapInput factorydefinitions.FactoryLayoutPayloadMapper,
	decodeFactory factorydefinitions.FactoryConfigJSONDecoder,
	normalizeAuthored func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error),
	encodeFactory func(*factorydefinitions.FactoryConfig) ([]byte, error),
	write func(string, *factorydefinitions.PreparedFactoryLayoutPayload, string) error,
	validate func(string) error,
	flatten factorydefinitions.FactoryLayoutFlattener,
	expand factorydefinitions.FactoryLayoutExpander,
	fileSystem factorydefinitions.PersistenceFileSystem,
	requireDefinitionDir factorydefinitions.DefinitionDirectoryRequirer,
	directories factorydefinitions.DirectoryReplacementStore,
) (authoringlayout.Service, error) {
	if validator == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: validator is required")
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
