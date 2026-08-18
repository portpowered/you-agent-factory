package climanifest_test

import (
	"os"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
)

type sourceStoreFunc func(string) ([]byte, error)

func (read sourceStoreFunc) Read(path string) ([]byte, error) { return read(path) }

func sourceStore() generatedartifacts.SourceStore { return sourceStoreFunc(os.ReadFile) }
