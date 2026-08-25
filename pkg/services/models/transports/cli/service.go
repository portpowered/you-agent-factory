package cli

import (
	"context"
	"io"
	"os"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

const modelsCLIInvokeHolder = "you-models-cli-invoke"

// ArtifactExporter is the narrow CLI transport capability used to copy a
// streamed invocation artifact to the operator-selected destination. The
// Models root does not publish this construction effect.
type ArtifactExporter interface {
	ExportInvocationArtifact(sourcePath, destinationPath string) error
}

// OutputTemporaryFile is the narrow writable handle used to stage one
// provider-neutral output before atomic publication.
type OutputTemporaryFile interface {
	io.Writer
	io.Closer
	Name() string
}

// OutputFileSystem is the exact filesystem effect required by explicit
// generic output mappings. The CLI owns validation and lifecycle; composition
// supplies the host implementation.
type OutputFileSystem interface {
	CreateTemp(string, string) (OutputTemporaryFile, error)
	Inspect(string) (os.FileInfo, error)
	Remove(string) error
	Rename(string, string) error
}

// InputFileReader is the exact filesystem effect used to bind one explicit
// generic CLI input. The Models CLI adapter owns parsing and validation; the
// composition boundary supplies the host reader. maxBytes is the inclusive
// content limit selected by this transport.
type InputFileReader func(context.Context, string, int64) ([]byte, error)

// InvokeRuntimeScope carries one opened Models runtime scope for invoke.
type InvokeRuntimeScope struct {
	Scope models.RuntimeScopeRef
	Close func(context.Context) error
}

// Config carries accepted Models-root collaborators for adapter construction.
type Config struct {
	Models           models.Service
	HTTP             clihttp.Protocol
	PullHTTP         clihttp.Protocol
	Artifacts        ArtifactExporter
	OutputFileSystem OutputFileSystem
	InputFileReader  InputFileReader
	OpenInvokeScope  func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	OpenCatalogScope func(context.Context) (InvokeRuntimeScope, error)
	Clock            func() time.Time
}

type rootService struct {
	models           models.Service
	http             clihttp.Protocol
	pullHTTP         clihttp.Protocol
	artifacts        ArtifactExporter
	outputFileSystem OutputFileSystem
	inputFileReader  InputFileReader
	openInvokeScope  func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	openCatalogScope func(context.Context) (InvokeRuntimeScope, error)
	now              func() time.Time
}

// NewService constructs the Models-owned CLI service from the accepted Models root.
func NewService(cfg Config) Service {
	if cfg.Models == nil {
		return nil
	}
	return &rootService{
		models:           cfg.Models,
		http:             cfg.HTTP,
		pullHTTP:         cfg.PullHTTP,
		artifacts:        cfg.Artifacts,
		outputFileSystem: cfg.OutputFileSystem,
		inputFileReader:  cfg.InputFileReader,
		openInvokeScope:  cfg.OpenInvokeScope,
		openCatalogScope: cfg.OpenCatalogScope,
		now:              cfg.Clock,
	}
}
