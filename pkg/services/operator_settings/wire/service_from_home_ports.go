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

// NewServiceFromHomePorts constructs the accepted Settings root from the
// filesystem and decoder ports Wire already injects for defaults resolution.
// logger is an optional trailing operation-logging abstraction; omitting it
// (or passing nil) resolves to a safe no-op.
func NewServiceFromHomePorts(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	providersRoot providers.Service,
	idGenerator operatorsettings.IDGenerator,
	logger ...logging.Logger,
) (operatorsettings.Service, error) {
	if files == nil {
		return nil, fmt.Errorf("operator settings filesystem is required")
	}
	if decode == nil {
		return nil, fmt.Errorf("operator settings decoder is required")
	}
	if providersRoot == nil {
		return nil, fmt.Errorf("operator settings providers root is required")
	}
	if idGenerator == nil {
		return nil, fmt.Errorf("operator settings ID generator is required")
	}
	document := documentwire.NewService(files, nil, decode, nil, nil)
	resolution, err := resolutionwire.NewService(providersRoot)
	if err != nil {
		return nil, err
	}
	return operatorservice.New(
		document,
		resolution,
		files,
		nil,
		decode,
		nil,
		idGenerator,
		firstLogger(logger),
	)
}
