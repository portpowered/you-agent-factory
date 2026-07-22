package models

import "io"

// InvocationArtifactFileSystem is the exact external filesystem effect used
// to export a streamed model-invocation artifact to a caller-owned path.
type InvocationArtifactFileSystem interface {
	Open(string) (io.ReadCloser, error)
	Create(string) (io.WriteCloser, error)
}

// InvocationArtifactExporter materializes a runtime-owned streamed artifact at
// a caller-selected destination without exposing filesystem effects to a
// transport.
type InvocationArtifactExporter interface {
	ExportInvocationArtifact(sourcePath, destinationPath string) error
}
