package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// NewServiceFromConfigDocument constructs the accepted Settings root from the
// ConfigDocumentService ports Wire already injects for configure composition.
// logger is the direct, required operation-logging abstraction; callers with
// no operation logging pass logging.NoopLogger{}.
func NewServiceFromConfigDocument(
	service operatorsettings.ConfigDocumentService,
	providersRoot providers.Service,
	idGenerator operatorsettings.IDGenerator,
	logger logging.Logger,
) (operatorsettings.Service, error) {
	if providersRoot == nil {
		return nil, fmt.Errorf("operator settings providers root is required")
	}
	owner := service.DocumentOwner
	if owner == nil {
		if service.Files == nil || service.CreateTemp == nil || service.Decoder == nil ||
			service.Encoder == nil || service.Providers == nil {
			return nil, fmt.Errorf("operator settings document ports are required")
		}
		if service.PreserveUnknownFields != nil {
			owner = NewDocumentOwnerWithPreserver(
				service.Files,
				service.CreateTemp,
				service.Decoder,
				service.Encoder,
				service.Providers,
				service.PreserveUnknownFields,
				service.DiagnosticDecoder,
			)
		} else {
			owner = NewDocumentOwner(service.Files, service.CreateTemp, service.Decoder, service.Encoder, service.Providers, service.DiagnosticDecoder)
		}
	}
	document, ok := owner.(settingsdocument.Service)
	if !ok {
		return nil, fmt.Errorf("operator settings document owner must implement the private document service")
	}
	if idGenerator == nil {
		return nil, fmt.Errorf("operator settings ID generator is required")
	}
	resolution, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		return nil, err
	}
	return operatorservice.New(
		document,
		resolution,
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		idGenerator,
		logger,
		service.DiagnosticDecoder,
	)
}
