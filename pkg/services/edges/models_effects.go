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

type HostProcessStartSpec struct {
	Command                 string
	Args, Env               []string
	WorkDir, HealthEndpoint string
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
