package inference

import (
	"fmt"
	"io"
)

// InertArtifactFileSystem satisfies Inference construction without opening
// files. Export fails until a real filesystem port is injected at export time.
type InertArtifactFileSystem struct{}

func (InertArtifactFileSystem) Open(string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("inference artifact open requires an explicit export filesystem")
}

func (InertArtifactFileSystem) Create(string) (io.WriteCloser, error) {
	return nil, fmt.Errorf("inference artifact create requires an explicit export filesystem")
}
