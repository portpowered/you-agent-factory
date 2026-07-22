package baseline

import (
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
)

type sourceStoreFunc func(string) ([]byte, error)

func (read sourceStoreFunc) Read(path string) ([]byte, error) { return read(path) }

func testSourceStore() generatedartifacts.SourceStore {
	return sourceStoreFunc(os.ReadFile)
}
