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
	files      operatorsettings.FileSystem
	createTemp operatorsettings.CreateTemporaryFile
	decoder    operatorsettings.ConfigDecoder
	encoder    operatorsettings.ConfigEncoder
	providers  operatorsettings.ProviderCatalog
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
) *Service {
	return &Service{
		files:      files,
		createTemp: createTemp,
		decoder:    decoder,
		encoder:    encoder,
		providers:  providers,
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
) operatorsettings.DocumentOwner {
	return New(files, createTemp, decoder, encoder, providers)
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
