package edges

import (
	"context"
	"io"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// PullMetric is the process-edge representation of one managed-model pull
// metric. Models converts this edge-owned value at its wire boundary.
type PullMetric struct {
	Name   string
	Labels map[string]string
}

type AssetMakeDirectories func(string, os.FileMode) error
type AssetInspectPath func(string) (os.FileInfo, error)
type AssetResolveHomeDirectory func() (string, error)
type AssetWriteFile func(string, []byte, os.FileMode) error
type AssetRenamePath func(string, string) error
type AssetRemovePath func(string) error
type AssetReadFile func(string) ([]byte, error)
type AssetReadDirectory func(string) ([]os.DirEntry, error)
type AssetCreateFile func(string) (io.WriteCloser, error)
type AssetOpenFile func(string) (io.ReadCloser, error)

// AssetStagingCoordination is the exact cancellation-aware ownership effect
// used to serialize cross-process asset staging. Models selects the identity
// and transaction boundary; the edge supplies only the lock lifecycle.
type AssetStagingCoordination interface {
	Lock(context.Context, string) (io.Closer, error)
}

// AssetStagingCoordinationFactory constructs the Models asset staging effect.
// The factory is injectable so root composition can report a missing platform
// effect as a construction failure instead of allowing a later request panic.
type AssetStagingCoordinationFactory func() (AssetStagingCoordination, error)

// ModelCLIOutputCreateTempFile is the host effect used by the Models CLI
// output publisher. The caller selects the directory, naming pattern, and
// publication lifecycle; the edge supplies only the writable handle.
type ModelCLIOutputCreateTempFile func(string, string) (interface {
	io.Writer
	io.Closer
	Name() string
}, error)

// ModelCLIInputReadFile is the host effect used to load one explicit generic
// CLI input. The CLI adapter owns mapping validation and media classification;
// this edge supplies only the file bytes. maxBytes is the inclusive content
// limit selected by the CLI transport, and implementations must honor ctx
// while preparing the content.
type ModelCLIInputReadFile func(context.Context, string, int64) ([]byte, error)

// ModelBackendArtifactSelectionRequest contains the safe host facts needed by
// the pinned backend publication selector. The edge owns manifest lookup;
// Models receives only the selected immutable archive facts.
type ModelBackendArtifactSelectionRequest struct {
	Backend         string
	Platform        models.AssetHostPlatform
	ProtocolVersion string
}

// ModelBackendArtifactSelection is the detached archive identity passed into
// Models asset preparation. Location is consumed only inside the composition
// graph and is never returned by the Models service.
type ModelBackendArtifactSelection struct {
	Name     string
	Location string
	Bytes    int64
	SHA256   string
}

type ModelResolveBackendArtifact func(
	context.Context,
	ModelBackendArtifactSelectionRequest,
) (ModelBackendArtifactSelection, error)

// ModelInvocationBackend is the replaceable backend operation effect used by
// functional fixtures and future managed-backend adapters. It returns only
// detached provider-neutral output facts; Models retains ownership of
// invocation status, output normalization, artifacts, and lease lifecycle.
type ModelInvocationBackend func(
	context.Context,
	models.InvokeModelRequest,
) ([]models.InferenceContent, []models.InferenceArtifact, error)

// ModelASRBackend is the typed ASR operation effect. Models owns the generic
// request validation and private LocalAI codec; this edge supplies only the
// detached protocol result and opaque artifact metadata.
type ModelASRBackend func(
	context.Context,
	models.ASRBackendRequest,
) (models.ASRBackendResponse, error)

// ModelEmbeddingBackend is the typed embedding operation effect. Models owns
// generic request validation, LocalAI codec mapping, output normalization, and
// lease lifecycle; this edge supplies only detached embedding facts.
type ModelEmbeddingBackend func(
	context.Context,
	models.EmbeddingBackendRequest,
) (models.EmbeddingBackendResponse, error)

type HostProcessStartSpec struct {
	Command                 string
	Args, Env               []string
	WorkDir, HealthEndpoint string
	Backend                 string
	ModelPath               string
	BackendFiles            []string
}

type RuntimeInspectFile func(string) (os.FileInfo, error)
type RuntimeTempDirectory func() string
type RuntimeCreateTempFile func(string, string) (interface {
	Close() error
	Name() string
}, error)

type ModelHostProtocolNegotiationRequest struct {
	ProtocolVersion string
	Backend         string
	ModelName       string
	Revision        string
	Platform        models.AssetHostPlatform
	ModelPath       string
}

type ModelHostProtocolNegotiationResult struct {
	ProtocolVersion string
	Backend         string
	Ready           bool
}

type ModelHostCompatibilityRequest struct {
	Backend   string
	ModelName string
	Revision  string
	Platform  models.AssetHostPlatform
}
