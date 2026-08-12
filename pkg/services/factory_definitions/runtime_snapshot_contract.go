package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ResolveRuntimeSnapshotRequest selects exactly one authored Factory source.
// FactoryDir and SourcePath are interchangeable path forms; Canonical is the
// in-memory canonical form. Requests contain only identity, execution-base,
// and invocation-context values, never runtime collaborators.
type ResolveRuntimeSnapshotRequest struct {
	FactoryDir       string
	SourcePath       string
	Canonical        []byte
	ExecutionBaseDir string
	Invocation       RuntimeSnapshotInvocationContext
}

// RuntimeSnapshotInvocationContext carries value-only context that belongs to
// the invocation which requested resolution. Definitions does not retain a
// session, runtime, provider, model, filesystem, or executor handle here.
type RuntimeSnapshotInvocationContext struct {
	FactorySessionID string
	WorkflowID       string
}

// ResolveRuntimeSnapshotResult carries one detached Runtime input. Every
// nested value is copied from the loaded source before it crosses the
// Definitions boundary.
type ResolveRuntimeSnapshotResult struct {
	Snapshot RuntimeSnapshot
}

// RuntimeSnapshot is the immutable-by-convention value projection consumed by
// Runtime activation. It deliberately contains authored/effective facts only:
// no service interfaces, callbacks, providers, model handles, executors,
// filesystems, or mutable loaded-source references are retained.
type RuntimeSnapshot struct {
	FactoryDir        string
	RuntimeBaseDir    string
	Invocation        RuntimeSnapshotInvocationContext
	DefinitionVersion *FactoryVersion
	EffectiveFactory  FactoryConfig
	Workers           []FactoryWorkerConfig
	Workstations      []FactoryWorkstationConfig
	AutomationSources []RuntimeAutomationSource
	PromptSources     []RuntimePromptSource
	BundledFiles      []PortableBundledFileReplacement
}

// CloneRuntimeSnapshot returns a detached copy suitable for crossing another
// service boundary. Runtime activation stores the copy it was given so a
// caller cannot change an already accepted activation by retaining one of the
// nested maps, slices, or pointers in its request.
func CloneRuntimeSnapshot(snapshot RuntimeSnapshot) (RuntimeSnapshot, error) {
	config, err := CloneFactoryConfig(&snapshot.EffectiveFactory)
	if err != nil {
		return RuntimeSnapshot{}, fmt.Errorf("clone runtime snapshot Factory: %w", err)
	}
	if config == nil {
		return RuntimeSnapshot{}, fmt.Errorf("clone runtime snapshot Factory: configuration is required")
	}
	for index := range config.Workers {
		if index < len(snapshot.EffectiveFactory.Workers) {
			config.Workers[index] = CloneWorkerConfig(snapshot.EffectiveFactory.Workers[index])
		}
	}
	for index := range config.Workstations {
		if index < len(snapshot.EffectiveFactory.Workstations) {
			config.Workstations[index] = CloneWorkstationConfig(snapshot.EffectiveFactory.Workstations[index])
		}
	}

	cloned := RuntimeSnapshot{
		FactoryDir:        snapshot.FactoryDir,
		RuntimeBaseDir:    snapshot.RuntimeBaseDir,
		Invocation:        snapshot.Invocation,
		EffectiveFactory:  *config,
		Workers:           make([]FactoryWorkerConfig, len(snapshot.Workers)),
		Workstations:      make([]FactoryWorkstationConfig, len(snapshot.Workstations)),
		AutomationSources: make([]RuntimeAutomationSource, len(snapshot.AutomationSources)),
		PromptSources:     append([]RuntimePromptSource(nil), snapshot.PromptSources...),
		BundledFiles:      append([]PortableBundledFileReplacement(nil), snapshot.BundledFiles...),
	}
	if snapshot.DefinitionVersion != nil {
		version := *snapshot.DefinitionVersion
		cloned.DefinitionVersion = &version
	}
	for index, worker := range snapshot.Workers {
		cloned.Workers[index] = CloneWorkerConfig(worker)
	}
	for index, workstation := range snapshot.Workstations {
		cloned.Workstations[index] = CloneWorkstationConfig(workstation)
	}
	for index, source := range snapshot.AutomationSources {
		cloned.AutomationSources[index] = source
		cloned.AutomationSources[index].Workstation = CloneWorkstationConfig(source.Workstation)
		if source.Worker != nil {
			worker := CloneWorkerConfig(*source.Worker)
			cloned.AutomationSources[index].Worker = &worker
		}
	}
	return cloned, nil
}

// RuntimeAutomationSource is the value-only automation definition associated
// with one effective workstation. Runtime and Automations use the embedded
// workstation/worker policy to create their own isolated live source state.
type RuntimeAutomationSource struct {
	ID              string
	Kind            RuntimeAutomationSourceKind
	WorkstationName string
	WorkerName      string
	Workstation     FactoryWorkstationConfig
	Worker          *FactoryWorkerConfig
	Schedule        string
	Every           string
	TriggerAtStart  bool
}

// RuntimeAutomationSourceKind classifies the authored trigger shape without
// exposing Automations implementation types.
type RuntimeAutomationSourceKind string

const (
	RuntimeAutomationSourceKindWorkstation RuntimeAutomationSourceKind = "WORKSTATION"
	RuntimeAutomationSourceKindCron        RuntimeAutomationSourceKind = "CRON"
	RuntimeAutomationSourceKindScript      RuntimeAutomationSourceKind = "SCRIPT"
	RuntimeAutomationSourceKindPoller      RuntimeAutomationSourceKind = "POLLER"
	RuntimeAutomationSourceKindHosted      RuntimeAutomationSourceKind = "HOSTED"
)

// RuntimePromptSource preserves fixed authored prompt identity separately from
// the effective Factory configuration, which intentionally omits source paths.
type RuntimePromptSource struct {
	Role       string
	Name       string
	Path       string
	IsTemplate bool
}

// RuntimeSnapshotDiagnosticCode classifies a typed resolution failure.
type RuntimeSnapshotDiagnosticCode string

const (
	RuntimeSnapshotDiagnosticInvalidRequest    RuntimeSnapshotDiagnosticCode = "invalid-request"
	RuntimeSnapshotDiagnosticInvalidDefinition RuntimeSnapshotDiagnosticCode = "invalid-definition"
	RuntimeSnapshotDiagnosticUnavailable       RuntimeSnapshotDiagnosticCode = "resolver-unavailable"
	RuntimeSnapshotDiagnosticCanceled          RuntimeSnapshotDiagnosticCode = "canceled"
)

// RuntimeSnapshotDiagnostic is a sensitive-safe failure fact that can be
// surfaced by transports without exposing loader implementation details.
type RuntimeSnapshotDiagnostic struct {
	Code    RuntimeSnapshotDiagnosticCode
	Field   string
	Message string
}

var (
	// ErrRuntimeSnapshotResolutionFailed is the stable umbrella for snapshot
	// resolution failures.
	ErrRuntimeSnapshotResolutionFailed = errors.New("runtime snapshot resolution failed")
	// ErrInvalidRuntimeSnapshotRequest reports missing or conflicting source
	// identity fields before any loader is invoked.
	ErrInvalidRuntimeSnapshotRequest = errors.New("invalid runtime snapshot request")
	// ErrInvalidRuntimeSnapshotDefinition reports a source that could not be
	// loaded, validated, or detached into an effective Factory.
	ErrInvalidRuntimeSnapshotDefinition = errors.New("invalid runtime snapshot definition")
	// ErrRuntimeSnapshotResolverUnavailable reports missing construction ports.
	ErrRuntimeSnapshotResolverUnavailable = errors.New("runtime snapshot resolver unavailable")
)

// RuntimeSnapshotResolutionError carries a stable diagnostic plus the typed
// Definitions error returned by source validation/loading when one exists.
type RuntimeSnapshotResolutionError struct {
	Diagnostic RuntimeSnapshotDiagnostic
	Cause      error
}

func (e *RuntimeSnapshotResolutionError) Error() string {
	if e == nil {
		return ErrRuntimeSnapshotResolutionFailed.Error()
	}
	message := strings.TrimSpace(e.Diagnostic.Message)
	if message == "" {
		message = string(e.Diagnostic.Code)
	}
	if e.Cause != nil {
		return fmt.Sprintf("%v: %s: %v", ErrRuntimeSnapshotResolutionFailed, message, e.Cause)
	}
	return fmt.Sprintf("%v: %s", ErrRuntimeSnapshotResolutionFailed, message)
}

func (e *RuntimeSnapshotResolutionError) Unwrap() error {
	if e == nil {
		return ErrRuntimeSnapshotResolutionFailed
	}
	return e.Cause
}

func (e *RuntimeSnapshotResolutionError) Is(target error) bool {
	if target == ErrRuntimeSnapshotResolutionFailed {
		return true
	}
	if e == nil {
		return false
	}
	switch e.Diagnostic.Code {
	case RuntimeSnapshotDiagnosticInvalidRequest:
		return target == ErrInvalidRuntimeSnapshotRequest
	case RuntimeSnapshotDiagnosticInvalidDefinition:
		return target == ErrInvalidRuntimeSnapshotDefinition
	case RuntimeSnapshotDiagnosticUnavailable:
		return target == ErrRuntimeSnapshotResolverUnavailable
	default:
		return false
	}
}

// RuntimeSnapshotOperation is the owner-composed operation used to attach
// snapshot resolution to the singular Definitions root.
type RuntimeSnapshotOperation func(
	context.Context,
	ResolveRuntimeSnapshotRequest,
) (ResolveRuntimeSnapshotResult, error)
