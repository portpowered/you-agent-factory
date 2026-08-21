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

type ModelHostProtocolNegotiator interface {
	Negotiate(context.Context, string, ModelHostProtocolNegotiationRequest) (ModelHostProtocolNegotiationResult, error)
}

type ModelHostGRPCDialer interface {
	Dial(context.Context, string) (ModelHostGRPCConnection, error)
}

type ModelHostGRPCConnection interface {
	Negotiate(context.Context, ModelHostProtocolNegotiationRequest) (ModelHostProtocolNegotiationResult, error)
	Close() error
}

type ModelHostCompatibilityRequest struct {
	Backend   string
	ModelName string
	Revision  string
	Platform  models.AssetHostPlatform
}

type ModelHostCompatibilityChecker interface {
	Check(context.Context, ModelHostCompatibilityRequest) error
}
