package cli

import (
	"context"

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

// InvokeRuntimeScope carries one opened Models runtime scope for invoke.
type InvokeRuntimeScope struct {
	Scope models.RuntimeScopeRef
	Close func(context.Context) error
}

// Config carries accepted Models-root collaborators for adapter construction.
type Config struct {
	Models           models.Service
	HTTP             clihttp.Protocol
	Artifacts        ArtifactExporter
	OpenInvokeScope  func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	OpenCatalogScope func(context.Context) (InvokeRuntimeScope, error)
}

type rootService struct {
	models           models.Service
	http             clihttp.Protocol
	artifacts        ArtifactExporter
	openInvokeScope  func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	openCatalogScope func(context.Context) (InvokeRuntimeScope, error)
}

// NewService constructs the Models-owned CLI service from the accepted Models root.
func NewService(cfg Config) Service {
	if cfg.Models == nil {
		return nil
	}
	return &rootService{
		models:           cfg.Models,
		http:             cfg.HTTP,
		artifacts:        cfg.Artifacts,
		openInvokeScope:  cfg.OpenInvokeScope,
		openCatalogScope: cfg.OpenCatalogScope,
	}
}
