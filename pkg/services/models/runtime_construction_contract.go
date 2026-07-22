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
type HostManagedProcess interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}
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
