package construct

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

func newServiceRoot(
	document operatorsettings.DocumentOwner,
	resolutionService resolution.Service,
) (operatorsettings.Service, error) {
	if document == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	if resolutionService == nil {
		return nil, fmt.Errorf("operator settings resolution service is required")
	}
	documentService, ok := document.(settingsdocument.Service)
	if !ok {
		return newCompositionRoot(document, resolutionService)
	}
	return operatorservice.New(documentService, resolutionService)
}

type compositionRoot struct {
	document   operatorsettings.DocumentOwner
	resolution resolution.Service
}

var _ operatorsettings.Service = (*compositionRoot)(nil)

func (root *compositionRoot) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	return root.document.LoadDocument(request)
}

func (root *compositionRoot) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return root.document.ApplyDocumentUpdate(request)
}

func (root *compositionRoot) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return root.resolution.ResolveEffective(request)
}

func newCompositionRoot(
	document operatorsettings.DocumentOwner,
	resolutionService resolution.Service,
) (operatorsettings.Service, error) {
	if document == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	if resolutionService == nil {
		return nil, fmt.Errorf("operator settings resolution service is required")
	}
	return &compositionRoot{
		document:   document,
		resolution: resolutionService,
	}, nil
}
