// Package wire constructs the Factory Definitions authoring_layout subservice
// from exact injected layout-parse, transform, and durable-write ports.
package wire

import (
	"fmt"

	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

// NewService constructs the private authoring_layout subservice from exact
// injected authoring ports. Callers must supply Dependencies; this constructor
// does not choose host filesystem adapters or take Wire/root construction
// ownership.
func NewService(deps authoringlayout.Dependencies) (authoringlayout.Service, error) {
	if deps.Validator == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: validator is required")
	}
	if deps.MapInput == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: payload mapper is required")
	}
	if deps.DecodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory decoder is required")
	}
	if deps.NormalizeAuthored == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: authored normalizer is required")
	}
	if deps.EncodeFactory == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: factory encoder is required")
	}
	if deps.Write == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout writer is required")
	}
	if deps.Validate == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout validator is required")
	}
	if deps.Flatten == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout flattener is required")
	}
	if deps.Expand == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: layout expander is required")
	}
	if deps.FileSystem == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: persistence filesystem is required")
	}
	if deps.RequireDefinitionDir == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: definition directory validator is required")
	}
	if deps.Directories == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: directory replacement store is required")
	}
	service := authoringlayoutservice.New(deps)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: implementation rejected its dependencies")
	}
	return service, nil
}
