package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// DocumentService and ResolutionService identify completed Settings owners.
type DocumentService = settingsdocument.Service
type ResolutionService = resolution.Service

// NewService binds completed owners. Nil classification and logger selection
// occur once at this supported construction boundary; construction is inert.
func NewService(
	document DocumentService,
	resolution ResolutionService,
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	idGenerator operatorsettings.IDGenerator,
	logger logging.Logger,
	diagnostics operatorsettings.ConfigDiagnosticsDecoder,
) (operatorsettings.Service, error) {
	if document == nil {
		return nil, fmt.Errorf("construct Operator Settings: document is required")
	}
	if resolution == nil {
		return nil, fmt.Errorf("construct Operator Settings: resolution is required")
	}
	return operatorservice.New(document, resolution, files, createTemp, decoder, encoder,
		idGenerator, logging.EnsureLogger(logger), diagnostics)
}

// NewResolutionService constructs the effective-resolution owner.
func NewResolutionService(providersRoot providers.Service) (ResolutionService, error) {
	return resolutionwire.NewService(providersRoot)
}

// NewCLIService binds the reusable Settings CLI adapter at composition.
func NewCLIService(root operatorsettings.Service) operatorsettingscli.Service {
	return operatorsettingscli.New(root)
}
