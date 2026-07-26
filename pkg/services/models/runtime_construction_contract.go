package models

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// ProcessDependencies are stable Models collaborators selected once by Wire.
// Factory Sessions inject only their runtime data through RuntimeBinding.
// These construction/effect ports are not the peer-facing source of truth for
// the published runtime-scope, catalog, assets, host/lease, or infer slices;
// peers consume those through the singular Service methods and plain
// request/result/typed-error vocabulary instead.
type ProcessDependencies struct {
	Logger      *zap.Logger
	Clock       func() time.Time
	PullMetrics PullMetricsRecorder
	HostLogger  HostDiagnosticLogger
	HostMetrics HostMetricsRecorder
	LocalHooks  LocalRuntimeHooks
}

type RuntimeAssetEndpoints struct{ BaseURL, APIBaseURL string }
type AssetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type AssetHostPlatform struct{ OperatingSystem, Architecture string }
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

// HostManagedProcess is a Wire/construction process handle for host supervisors.
// It is not a peer-facing host/lease contract; peers use Service.InspectRuntime,
// AcquireLease, and ReleaseLease with HostLease values instead.
type HostManagedProcess interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}

// HostProcessLauncher is a Wire/construction port that starts supervised host
// processes. Peers must not treat it as the published host/lease root slice;
// that slice stays on the singular Service methods and HostLease vocabulary.
type HostProcessLauncher interface {
	Start(context.Context, HostProcessStartSpec) (HostManagedProcess, error)
}
type HostHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type HostTimer interface {
	C() <-chan time.Time
	Stop() bool
}
type HostClock interface {
	Now() time.Time
	NewTimer(time.Duration) HostTimer
}

type RuntimeHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}
type RuntimeTempFile interface {
	Close() error
	Name() string
}
type RuntimeInspectFile func(string) (os.FileInfo, error)
type RuntimeTempDirectory func() string
type RuntimeCreateTempFile func(string, string) (RuntimeTempFile, error)
