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
	Logger                     *zap.Logger
	Clock                      func() time.Time
	PullMetrics                PullMetricsRecorder
	HostLogger                 HostDiagnosticLogger
	HostMetrics                HostMetricsRecorder
	LocalHooks                 LocalRuntimeHooks
	ResolveHuggingFaceRevision func(context.Context, string) (string, error)
	ResolveBackendArtifact     BackendArtifactResolver
	BackendArtifactPlatform    models.AssetHostPlatform
}

// BackendArtifactSelectionRequest contains only the facts needed to select a
// pinned backend archive. The selector owns the immutable publication manifest;
// Models receives detached archive facts and never exposes that manifest.
type BackendArtifactSelectionRequest struct {
	Backend         string
	Platform        models.AssetHostPlatform
	ProtocolVersion string
}

// BackendArtifactSelection is the provider-neutral archive identity consumed
// by the private asset preparation seam. Location is retained only inside the
// Models implementation and is never copied to a public result or error.
type BackendArtifactSelection struct {
	Name     string
	Location string
	Bytes    int64
	SHA256   string
}

// BackendArtifactResolver selects one immutable backend archive for a managed
// host. It is an injected effect so tests can use deterministic manifests and
// production can obtain the published P3 artifact set without live probing.
type BackendArtifactResolver func(
	context.Context,
	BackendArtifactSelectionRequest,
) (BackendArtifactSelection, error)

type AssetHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type AssetMakeDirectories func(string, os.FileMode) error
type AssetInspectPath func(string) (os.FileInfo, error)
type AssetResolveHomeDirectory func() (string, error)
type AssetResolveEnvironment func(string) string
type AssetWriteFile func(string, []byte, os.FileMode) error
type AssetRenamePath func(string, string) error
type AssetRemovePath func(string) error
type AssetReadFile func(string) ([]byte, error)
type AssetReadDirectory func(string) ([]os.DirEntry, error)
type AssetCreateFile func(string) (io.WriteCloser, error)
type AssetOpenFile func(string) (io.ReadCloser, error)

// AssetStagingCoordination is the exact cancellation-aware ownership effect
// used to serialize cross-process asset staging. Models selects the identity
// and transaction boundary; the effect only owns the filesystem lock
// lifecycle.
type AssetStagingCoordination interface {
	Lock(context.Context, string) (io.Closer, error)
}

type HostProcessStartSpec struct {
	Command                 string
	Args, Env               []string
	WorkDir, HealthEndpoint string
	Backend                 string
	ModelPath               string
	ModelFiles              []string
	BackendFiles            []string
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

// PinnedHostProtocolVersion is the protocol revision negotiated by managed
// LocalAI backend processes. The protocol representation stays behind the
// Models effects seam; callers never import generated backend types.
const PinnedHostProtocolVersion = "localai-backend-v1"

// HostProtocolNegotiationRequest contains only provider-neutral facts needed
// to negotiate a managed backend. It deliberately excludes endpoints,
// credentials, cache paths, process handles, and backend-native messages.
type HostProtocolNegotiationRequest struct {
	ProtocolVersion string
	Backend         string
	ModelName       string
	Revision        string
	Platform        models.AssetHostPlatform
	ModelPath       string
	ModelFiles      []string
}

// HostProtocolNegotiationResult is the detached result of one pinned
// readiness negotiation.
type HostProtocolNegotiationResult struct {
	ProtocolVersion string
	Backend         string
	Ready           bool
}

// HostProtocolNegotiator performs one readiness negotiation against a
// process-owned endpoint. Implementations may use gRPC or a deterministic
// protocol fixture, but LocalAI-native types remain private to the adapter.
type HostProtocolNegotiator interface {
	Negotiate(context.Context, string, HostProtocolNegotiationRequest) (HostProtocolNegotiationResult, error)
}

// HostGRPCDialer and HostGRPCConnection are the narrow policy-free effects
// needed by PinnedGRPCNegotiator. The generated protocol client, channel, and
// connection lifecycle remain behind this internal package boundary.
type HostGRPCDialer interface {
	Dial(context.Context, string) (HostGRPCConnection, error)
}

type HostGRPCConnection interface {
	Negotiate(context.Context, HostProtocolNegotiationRequest) (HostProtocolNegotiationResult, error)
	Close() error
}

// PinnedGRPCNegotiator adapts a policy-free gRPC dialer to the Models host
// protocol seam. It owns no endpoint, retry, timeout, or backend policy.
type PinnedGRPCNegotiator struct {
	Dialer HostGRPCDialer
}

func (negotiator PinnedGRPCNegotiator) Negotiate(
	ctx context.Context,
	endpoint string,
	request HostProtocolNegotiationRequest,
) (HostProtocolNegotiationResult, error) {
	if negotiator.Dialer == nil {
		return HostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	connection, err := negotiator.Dialer.Dial(ctx, endpoint)
	if err != nil {
		return HostProtocolNegotiationResult{}, err
	}
	if connection == nil {
		return HostProtocolNegotiationResult{}, models.ErrHostProtocolIncompatible
	}
	defer func() { _ = connection.Close() }()
	return connection.Negotiate(ctx, request)
}

// HostCompatibilityRequest carries provider-neutral compatibility facts to a
// platform/accelerator policy implementation.
type HostCompatibilityRequest struct {
	Backend   string
	ModelName string
	Revision  string
	Platform  models.AssetHostPlatform
}

// HostCompatibilityChecker validates platform and accelerator support before
// a managed process starts.
type HostCompatibilityChecker interface {
	Check(context.Context, HostCompatibilityRequest) error
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
