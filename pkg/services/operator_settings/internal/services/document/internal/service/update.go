package service

import (
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (service *Service) applyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	loaded, err := service.loadDocument(operatorsettings.LoadDocumentRequest{Path: request.Path})
	if err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}

	path := loaded.Path
	document := loaded.Document

	expected := strings.TrimSpace(request.ExpectedBackendScope)
	if expected != "" && document.BackendScopeID != expected {
		return operatorsettings.ApplyDocumentUpdateResult{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindConflict,
			Message: "backend scope mismatch",
			Path:    path,
		}
	}

	updated, err := service.mergeProviderModelUpdate(document, request.ProviderModel)
	if err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}

	return operatorsettings.ApplyDocumentUpdateResult{
		Document:  updated,
		Path:      path,
		Persisted: false,
	}, nil
}

func (service *Service) mergeProviderModelUpdate(
	document operatorsettings.Document,
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	validated, err := service.validateProviderModelUpdate(update)
	if err != nil {
		return operatorsettings.Document{}, err
	}

	config := configFromDocument(document)
	if validated.Provider != nil {
		config.Defaults.WorkerModelProvider = *validated.Provider
	}
	if validated.Model != nil {
		config.Defaults.WorkerModel = strings.TrimSpace(*validated.Model)
	}

	normalized, err := config.Normalize()
	if err != nil {
		return operatorsettings.Document{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindMalformed,
			Message: err.Error(),
		}
	}
	return documentFromConfig(normalized), nil
}

func (service *Service) validateProviderModelUpdate(
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.DocumentProviderModelUpdate, error) {
	if update.Provider == nil {
		return update, nil
	}
	provider := strings.TrimSpace(*update.Provider)
	if provider == "" {
		return operatorsettings.DocumentProviderModelUpdate{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindMalformed,
			Message: "worker model provider is required",
		}
	}
	if service.providers == nil {
		return operatorsettings.DocumentProviderModelUpdate{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindMalformed,
			Message: "operator provider catalog is required",
		}
	}
	canonical, ok := service.providers(provider)
	canonical = strings.TrimSpace(canonical)
	if !ok || canonical == "" {
		return operatorsettings.DocumentProviderModelUpdate{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindUnsupported,
			Message: fmt.Sprintf("unsupported worker model provider %q", provider),
		}
	}
	update.Provider = &canonical
	return update, nil
}
