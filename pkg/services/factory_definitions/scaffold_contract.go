package factorydefinitions

import (
	"io"
	"io/fs"

	contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
)

const DefaultFactoryInputType = contracts.DefaultFactoryInputType

type ScaffoldConfig = contracts.ScaffoldConfig
type ScaffoldInitializer = contracts.ScaffoldInitializer

// ScaffoldFileSystem is the exact filesystem effect required to materialize a
// newly authored Factory scaffold. Factory Definitions owns the scaffold
// layout and overwrite policy; the injected adapter only performs the selected
// filesystem operations.
type ScaffoldFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// ScaffoldOutput is the process output selected at the composition edge when
// a scaffold request does not supply an invocation-local output stream.
type ScaffoldOutput interface {
	io.Writer
}
