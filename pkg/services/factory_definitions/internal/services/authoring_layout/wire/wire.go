// Package wire constructs the Factory Definitions authoring_layout subservice
// from exact injected effect ports.
package wire

import (
	"fmt"

	authoringlayout "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout"
	authoringlayoutservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/internal/service"
)

// NewService constructs the private authoring_layout Service from exact ports.
func NewService(ports authoringlayout.Ports) (authoringlayout.Service, error) {
	if ports.Prepare == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: prepare port is required")
	}
	if ports.Flatten == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: flatten port is required")
	}
	if ports.Expand == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: expand port is required")
	}
	if ports.Create == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: create port is required")
	}
	if ports.Replace == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: replace port is required")
	}
	service := authoringlayoutservice.New(ports)
	if service == nil {
		return nil, fmt.Errorf("construct Factory Definitions authoring_layout: implementation rejected its dependencies")
	}
	return service, nil
}
