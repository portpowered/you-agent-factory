package operatorsettings

import (
	"os"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

var testFiles platformfilesystem.Local
var testIDGenerator IDGenerator = uuid.NewString
var testCreateTemp CreateTemporaryFile = func(dir, pattern string) (TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func ensureTestBackendScope(path string) (ResolvedBackendScope, error) {
	return EnsureLocalBackendScope(testFiles, testCreateTemp, testIDGenerator, decodeTestConfig, encodeTestConfig, path)
}
