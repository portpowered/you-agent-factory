// Package service implements the published Operator Settings root Service by
// delegating effective resolution to the parent-private resolution subservice.
package service

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

// Service fulfills the published Operator Settings root contract.
type Service struct {
	resolution resolution.Service
}

var _ operatorsettings.Service = (*Service)(nil)

// New constructs an inert Operator Settings root facade over the private
// resolution capability. Document operations remain outside this packet until
// the document subservice is composed.
func New(resolutionService resolution.Service) (operatorsettings.Service, error) {
	if resolutionService == nil {
		return nil, fmt.Errorf("construct Operator Settings: resolution is required")
	}
	return &Service{resolution: resolutionService}, nil
}

func (s *Service) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
		Kind:    operatorsettings.DocumentFailureKindUnsupported,
		Message: "document subservice is not composed",
		Path:    request.Path,
	}
}

func (s *Service) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
		Kind:    operatorsettings.DocumentFailureKindUnsupported,
		Message: "document subservice is not composed",
		Path:    request.Path,
	}
}

func (s *Service) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return s.resolution.ResolveEffective(request)
}
