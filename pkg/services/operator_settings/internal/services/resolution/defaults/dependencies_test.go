package settingsresolution_test

import (
	"os"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var testFiles platformfilesystem.Local

var testCreateTemp operatorsettings.CreateTemporaryFile = func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

var testIDGenerator operatorsettings.IDGenerator = uuid.NewString
