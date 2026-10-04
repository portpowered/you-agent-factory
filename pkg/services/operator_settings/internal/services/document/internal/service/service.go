package service

import (
	"context"
	"errors"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
)

var errDocumentOwnerUnavailable = errors.New("operator settings document owner is unavailable")

// Service keeps document load/update/persist behavior behind the
// Operator Settings-owned document capability.
type Service struct {
	files             operatorsettings.FileSystem
	createTemp        operatorsettings.CreateTemporaryFile
	decoder           operatorsettings.ConfigDecoder
	encoder           operatorsettings.ConfigEncoder
	providers         operatorsettings.ProviderCatalog
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder
	preserveUnknown   operatorsettings.ConfigDocumentPreserver
}

var _ settingsdocument.Service = (*Service)(nil)

// NewWithPreserver constructs the private document owner with the optional
// compatibility-preservation port used by production Operator Settings.
func NewWithPreserver(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder,
) *Service {
	return newService(files, createTemp, decoder, encoder, providers, preserveUnknown, diagnosticDecoder)
}

func newService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder,
) *Service {
	return &Service{
		files:             files,
		createTemp:        createTemp,
		decoder:           decoder,
		encoder:           encoder,
		providers:         providers,
		diagnosticDecoder: diagnosticDecoder,
		preserveUnknown:   preserveUnknown,
	}
}

func (service *Service) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if service == nil {
		return operatorsettings.LoadDocumentResult{}, errDocumentOwnerUnavailable
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.LoadDocumentResult{}, err
	}
	return service.loadDocument(request)
}

func (service *Service) MergeDocumentProviderModel(
	document operatorsettings.Document,
	update operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	if service == nil {
		return operatorsettings.Document{}, errDocumentOwnerUnavailable
	}
	return service.mergeProviderModelUpdate(document, update)
}

func (service *Service) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if service == nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, errDocumentOwnerUnavailable
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, err
	}
	return service.applyDocumentUpdate(request)
}

func (service *Service) PersistDocument(
	ctx context.Context,
	request operatorsettings.PersistDocumentRequest,
) error {
	if service == nil {
		return errDocumentOwnerUnavailable
	}
	if err := request.Validate(); err != nil {
		return err
	}
	return service.persistDocument(ctx, request)
}
