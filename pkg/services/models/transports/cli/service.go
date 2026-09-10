package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
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

// InvokeScopeRequest carries the stable CLI invoke configuration plus an
// optional invocation-local managed-model cache selection. Keeping the cache
// value in this separate request preserves the positional source shape of the
// exported InvokeConfig used by existing embedded callers.
type InvokeScopeRequest struct {
	Config        InvokeConfig
	ModelCacheDir string
	// Offline requires model and backend resolution to use only verified local
	// cache artifacts. It is carried beside InvokeConfig so the latter retains
	// its historical positional source shape for embedded callers.
	Offline bool
}

// ModelCacheInvoker is an optional additive capability for Models CLI
// services. Existing callers continue to use Service.Invoke; the production
// command handler uses this capability only when an invocation-local cache was
// selected.
type ModelCacheInvoker interface {
	InvokeWithModelCache(InvokeConfig, string) error
}

// InvokeScopeInvoker is an optional additive capability for the Models CLI
// service. It carries invocation-local policy beside the compatibility-stable
// InvokeConfig, allowing the command handler to express offline mode without
// changing existing positional callers.
type InvokeScopeInvoker interface {
	InvokeWithScope(InvokeScopeRequest) error
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
	models                    models.Service
	http                      clihttp.Protocol
	pullHTTP                  clihttp.Protocol
	artifacts                 ArtifactExporter
	outputFileSystem          OutputFileSystem
	inputFileReader           InputFileReader
	openInvokeScope           func(context.Context, InvokeConfig) (InvokeRuntimeScope, error)
	openInvokeScopeWithCache  func(context.Context, InvokeScopeRequest) (InvokeRuntimeScope, error)
	openCatalogScope          func(context.Context) (InvokeRuntimeScope, error)
	openCatalogScopeWithCache func(context.Context, CatalogScopeRequest) (InvokeRuntimeScope, error)
	now                       func() time.Time
}

// NewService constructs the Models-owned CLI service from the accepted Models root.
func NewService(cfg Config) Service {
	return newService(cfg, nil)
}

func (service *rootService) Invoke(cfg InvokeConfig) error {
	return service.InvokeWithScope(InvokeScopeRequest{Config: cfg})
}

func (service *rootService) InvokeWithModelCache(cfg InvokeConfig, modelCacheDir string) error {
	return service.InvokeWithScope(InvokeScopeRequest{Config: cfg, ModelCacheDir: modelCacheDir})
}

func (service *rootService) InvokeWithScope(request InvokeScopeRequest) error {
	return service.invoke(request)
}

func (service *rootService) openInvokeScopeForRequest(
	request InvokeScopeRequest,
) (InvokeRuntimeScope, error) {
	if service.openInvokeScopeWithCache != nil {
		return service.openInvokeScopeWithCache(request.Config.Context, request)
	}
	if strings.TrimSpace(request.ModelCacheDir) != "" {
		return InvokeRuntimeScope{}, fmt.Errorf("models invoke runtime scope opener does not support invocation-local model cache selection")
	}
	if service.openInvokeScope == nil {
		return InvokeRuntimeScope{}, fmt.Errorf("models invoke runtime scope opener is required")
	}
	return service.openInvokeScope(request.Config.Context, request.Config)
}

func newService(
	cfg Config,
	openInvokeScopeWithCache func(context.Context, InvokeScopeRequest) (InvokeRuntimeScope, error),
) Service {
	if cfg.Models == nil {
		return nil
	}
	return &rootService{
		models:                   cfg.Models,
		http:                     cfg.HTTP,
		pullHTTP:                 cfg.PullHTTP,
		artifacts:                cfg.Artifacts,
		outputFileSystem:         cfg.OutputFileSystem,
		inputFileReader:          cfg.InputFileReader,
		openInvokeScope:          cfg.OpenInvokeScope,
		openInvokeScopeWithCache: openInvokeScopeWithCache,
		openCatalogScope:         cfg.OpenCatalogScope,
		now:                      cfg.Clock,
	}
}
