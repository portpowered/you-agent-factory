// Package service implements the published Operator Settings root Service by
// delegating document operations and effective resolution to parent-private
// owner subservices.
package service

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

// Service fulfills the published Operator Settings root contract.
type Service struct {
	document   settingsdocument.Service
	resolution resolution.Service
}

var _ operatorsettings.Service = (*Service)(nil)

// New constructs an inert Operator Settings root facade over the private
// document and resolution capabilities.
func New(
	documentService settingsdocument.Service,
	resolutionService resolution.Service,
) (operatorsettings.Service, error) {
	if documentService == nil {
		return nil, fmt.Errorf("construct Operator Settings: document is required")
	}
	if resolutionService == nil {
		return nil, fmt.Errorf("construct Operator Settings: resolution is required")
	}
	return &Service{
		document:   documentService,
		resolution: resolutionService,
	}, nil
}

func (s *Service) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	return s.document.LoadDocument(request)
}

func (s *Service) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return s.document.ApplyDocumentUpdate(request)
}

func (s *Service) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return s.resolution.ResolveEffective(request)
}
