package http

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// rootFake is a focused Operator Settings root fake for adapter-edge tests. It
// avoids constructing document persistence, filesystem codecs, resolution Wire
// graphs, or service-local composition.
type rootFake struct {
	operatorsettings.Service

	loadDocument        func(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error)
	applyDocumentUpdate func(operatorsettings.ApplyDocumentUpdateRequest) (operatorsettings.ApplyDocumentUpdateResult, error)
	resolveEffective    func(operatorsettings.ResolveEffectiveRequest) (operatorsettings.ResolveEffectiveResult, error)
}

func (fake *rootFake) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if fake.loadDocument != nil {
		return fake.loadDocument(request)
	}
	return operatorsettings.LoadDocumentResult{}, operatorsettings.ErrDocumentNotFound
}

func (fake *rootFake) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if fake.applyDocumentUpdate != nil {
		return fake.applyDocumentUpdate(request)
	}
	return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.ErrDocumentMalformed
}

func (fake *rootFake) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if fake.resolveEffective != nil {
		return fake.resolveEffective(request)
	}
	return operatorsettings.ResolveEffectiveResult{}, operatorsettings.ErrResolutionInvalidInput
}
