package operatorsettingsservicewire

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

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

func newResolutionService() (resolution.Service, error) {
	providersRoot, err := providerswire.NewService()
	if err != nil {
		return nil, fmt.Errorf("construct providers root: %w", err)
	}
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		return nil, fmt.Errorf("construct resolution service: %w", err)
	}
	return resolutionService, nil
}
