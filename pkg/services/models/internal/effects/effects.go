// Package effects contains the private construction and external-effect ports
// used while assembling the Models service. They are deliberately kept out of
// the public Models contract; peers consume models.Service and detached
// request/result values instead.
package effects

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

type PullMetric struct {
	Name   string
	Labels map[string]string
}

type PullMetricsRecorder interface {
	RecordModelPullMetric(PullMetric)
}

// SlotFacts captures Runtime Host readiness and configured capacity for one
// scoped model slot. The lease owner consumes it through the private effects
// boundary; it is not part of the public Models contract.
type SlotFacts struct {
	Readiness       models.ReadinessState
	Capacity        int
	ContendedHolder string
}

// SlotFactsProvider supplies host-owned readiness and capacity facts for lease
// acquisition.
type SlotFactsProvider interface {
	SlotFacts(context.Context, models.RuntimeScopeRef, string) (SlotFacts, error)
}

// SlotCapacityCoordinator notifies Runtime Host when nested lease capacity
// holders change.
type SlotCapacityCoordinator interface {
	OnLeaseCapacityAcquired(models.RuntimeScopeRef, string)
	OnLeaseCapacityReleased(models.RuntimeScopeRef, string)
}

// CoordinatorBindable accepts a Runtime Host capacity coordinator after
// construction so wire can bind holder-aware cleanup without a cycle.
type CoordinatorBindable interface {
	BindSlotCapacityCoordinator(SlotCapacityCoordinator)
}

// UnconfiguredSlotFacts reports runtime-not-ready until Runtime Host wires a
// live facts adapter during construction.
type UnconfiguredSlotFacts struct{}

func (UnconfiguredSlotFacts) SlotFacts(
	context.Context,
	models.RuntimeScopeRef,
	string,
) (SlotFacts, error) {
	return SlotFacts{}, models.ErrHostRuntimeNotReady
}

type HostDiagnosticLogger interface {
	Info(string, map[string]string)
	Warn(string, map[string]string)
}

type HostMetricsRecorder interface {
	RecordMetric(string, map[string]string)
}

type LocalRuntimeHooks struct {
	MarkResourceWaitStarted  func(context.Context, time.Time)
	MarkResourceWaitFinished func(context.Context, time.Time, bool)
	MarkLoadRequested        func(context.Context, time.Time)
	MarkLoadFinished         func(context.Context, time.Time)
	MarkLoadReused           func(context.Context)
}

type ProcessDependencies struct {
	Logger      *zap.Logger
	Clock       func() time.Time
	PullMetrics PullMetricsRecorder
	HostLogger  HostDiagnosticLogger
	HostMetrics HostMetricsRecorder
	LocalHooks  LocalRuntimeHooks
}

type AssetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type AssetMakeDirectories func(string, os.FileMode) error
type AssetInspectPath func(string) (os.FileInfo, error)
type AssetResolveHomeDirectory func() (string, error)
type AssetWriteFile func(string, []byte, os.FileMode) error
type AssetRenamePath func(string, string) error
type AssetRemovePath func(string) error

// AssetRemoveTreeState is the explicit completion vocabulary for the private
// Models removal effect. The canonical composition boundary maps the platform
// result into this service-owned value rather than coupling platform code to
// Models.
type AssetRemoveTreeState string

const (
	AssetRemoveTreeNotAttempted AssetRemoveTreeState = "NOT_ATTEMPTED"
	AssetRemoveTreeAbsent       AssetRemoveTreeState = "ABSENT"
	AssetRemoveTreeRemoved      AssetRemoveTreeState = "REMOVED"
	AssetRemoveTreeRemaining    AssetRemoveTreeState = "REMAINING"
	AssetRemoveTreeUnknown      AssetRemoveTreeState = "UNKNOWN"
)

// AssetRemoveTreeResult records one platform removal attempt without exposing
// handles or filesystem-specific identity data to the Models service.
type AssetRemoveTreeResult struct {
	State AssetRemoveTreeState
}

// AssetRemoveTree removes one model-cache directory beneath a selected cache
// parent through a platform-owned path-security boundary.
type AssetRemoveTree func(context.Context, string, string) (AssetRemoveTreeResult, error)
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

type InvocationArtifactFileSystem interface {
	Open(string) (io.ReadCloser, error)
	Create(string) (io.WriteCloser, error)
}

type InvocationArtifactExporter interface {
	ExportInvocationArtifact(sourcePath, destinationPath string) error
}
