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

// New constructs the private document owner from injected filesystem, codec, and
// provider-catalog ports. Construction performs no filesystem, temp-file, or
// codec work.
func New(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) *Service {
	return newService(files, createTemp, decoder, encoder, providers, nil, diagnosticDecoders...)
}

// NewWithPreserver constructs the private document owner with the optional
// compatibility-preservation port used by production Operator Settings.
func NewWithPreserver(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) *Service {
	return newService(files, createTemp, decoder, encoder, providers, preserveUnknown, diagnosticDecoders...)
}

func newService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) *Service {
	var diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder
	if len(diagnosticDecoders) > 0 {
		diagnosticDecoder = diagnosticDecoders[0]
	}
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

// RebindDocumentOwner returns a new owner over the current compatibility
// adapter ports. ConfigDocumentService exposes those ports for legacy callers;
// rebinding keeps mutations to that adapter view from bypassing the nested
// owner while preserving custom owners that do not implement this hook.
func (service *Service) RebindDocumentOwner(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.DocumentOwner {
	return New(files, createTemp, decoder, encoder, providers, diagnosticDecoders...)
}

// RebindDocumentOwnerWithPreserver returns a new owner over compatibility
// adapter ports while retaining the production unknown-field preservation
// policy.
func (service *Service) RebindDocumentOwnerWithPreserver(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.DocumentOwner {
	return NewWithPreserver(files, createTemp, decoder, encoder, providers, preserveUnknown, diagnosticDecoders...)
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
