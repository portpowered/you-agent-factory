// Package wire is the Operator Settings service composition boundary.
//
// Wire performs construction only, returns the singular operatorsettings.Service
// root interface, and starts no lifecycle components. Parent-private document
// and resolution owner wiring stays inside the owner service assembly path;
// peers depend on Service rather than owner internals or construction ports.
package wire

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/service"
	documentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
	resolutionwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/wire"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// firstLogger returns the first optional logger supplied to a Settings wire
// constructor, or nil when omitted. operatorservice.New resolves a nil logger
// to a safe no-op, so this keeps every Settings wire constructor's logger
// parameter optional without duplicating that fallback at each call site.
func firstLogger(logger []logging.Logger) logging.Logger {
	if len(logger) == 0 {
		return nil
	}
	return logger[0]
}

// NewService constructs an inert Operator Settings root from construction and
// process-edge ports. It composes the accepted root through parent-private
// document and resolution owners without publishing owner types on the returned
// peer surface. logger is an optional trailing operation-logging abstraction;
// omitting it (or passing nil) resolves to a safe no-op.
func NewService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providersCatalog operatorsettings.ProviderCatalog,
	providersRoot providers.Service,
	idGenerator operatorsettings.IDGenerator,
	logger ...logging.Logger,
) (operatorsettings.Service, error) {
	if err := validateNewServiceInputs(
		files,
		createTemp,
		decoder,
		encoder,
		providersCatalog,
		providersRoot,
	); err != nil {
		return nil, err
	}
	if idGenerator == nil {
		return nil, fmt.Errorf("construct Operator Settings: ID generator is required")
	}

	documentService := documentwire.NewService(
		files,
		createTemp,
		decoder,
		encoder,
		providersCatalog,
	)
	resolutionService, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		return nil, err
	}
	return operatorservice.New(
		documentService,
		resolutionService,
		files,
		createTemp,
		decoder,
		encoder,
		idGenerator,
		firstLogger(logger),
	)
}

func validateNewServiceInputs(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providersCatalog operatorsettings.ProviderCatalog,
	providersRoot providers.Service,
) error {
	if files == nil {
		return fmt.Errorf("construct Operator Settings: filesystem is required")
	}
	if createTemp == nil {
		return fmt.Errorf("construct Operator Settings: create temporary file is required")
	}
	if decoder == nil {
		return fmt.Errorf("construct Operator Settings: config decoder is required")
	}
	if encoder == nil {
		return fmt.Errorf("construct Operator Settings: config encoder is required")
	}
	if providersCatalog == nil {
		return fmt.Errorf("construct Operator Settings: provider catalog is required")
	}
	if providersRoot == nil {
		return fmt.Errorf("construct Operator Settings: providers root is required")
	}
	return nil
}
