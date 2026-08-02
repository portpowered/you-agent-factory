package identityinputinventory

import (
	"os"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var testFiles platformfilesystem.Local
var testIDGenerator operatorsettings.IDGenerator = uuid.NewString
var testCreateTemp operatorsettings.CreateTemporaryFile = func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func decodeTestConfig(data []byte) (operatorsettings.Config, error) {
	return globalconfigmapping.Decode(data)
}

func encodeTestConfig(config operatorsettings.Config) ([]byte, error) {
	return globalconfigmapping.Encode(config)
}

func ensureTestBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.EnsureLocalBackendScope(
		testFiles,
		testCreateTemp,
		testIDGenerator,
		decodeTestConfig,
		encodeTestConfig,
		path,
	)
}
