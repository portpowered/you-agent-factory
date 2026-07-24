package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Service is the singular Factory Definitions root contract. Cross-service
// peers depend on this interface for Definitions authority. Later CTR-DEF
// slices publish catalog, authoring, compile, validate, snapshot, and
// distribute operations on this same Service using plain request, result,
// value, and typed-error contracts rather than implementation-package types.
type Service interface {
	ActivateNamedFactory(context.Context, string) error
	Save(context.Context, string, SaveMode, EditableFactory) (EditableFactory, error)
	GetCurrentNamedFactory(context.Context) (*FactorySnapshot, error)
	GetCurrentFactoryForSession(context.Context, string) (EditableFactory, error)
	CurrentFactoryDefinitionVersionAtRoot(string, string) (FactoryVersion, error)

	// Catalog slice: list, get/resolve, delete, and current-pointer read/write.
	ListNamedFactories(context.Context, ListNamedFactoriesRequest) (ListNamedFactoriesResult, error)
	GetNamedFactory(context.Context, GetNamedFactoryRequest) (GetNamedFactoryResult, error)
	ResolveNamedFactory(context.Context, ResolveNamedFactoryRequest) (ResolveNamedFactoryResult, error)
	DeleteNamedFactory(context.Context, DeleteNamedFactoryRequest) (DeleteNamedFactoryResult, error)
	GetCurrentFactoryPointer(context.Context, GetCurrentFactoryPointerRequest) (GetCurrentFactoryPointerResult, error)
	SetCurrentFactoryPointer(context.Context, SetCurrentFactoryPointerRequest) (SetCurrentFactoryPointerResult, error)

	// Authoring slice: parse/prepare, flatten, expand, create, and replace.
	PrepareFactoryLayout(context.Context, PrepareFactoryLayoutRequest) (PrepareFactoryLayoutResult, error)
	FlattenFactoryLayout(context.Context, FlattenFactoryLayoutRequest) (FlattenFactoryLayoutResult, error)
	ExpandFactoryLayout(context.Context, ExpandFactoryLayoutRequest) (ExpandFactoryLayoutResult, error)
	CreateNamedFactory(context.Context, CreateNamedFactoryRequest) (CreateNamedFactoryResult, error)
	ReplaceNamedFactory(context.Context, ReplaceNamedFactoryRequest) (ReplaceNamedFactoryResult, error)

	// Compile slice: authored/canonical source into one normalized effective source.
	CompileEffectiveFactorySource(context.Context, CompileEffectiveFactorySourceRequest) (CompileEffectiveFactorySourceResult, error)

	// Validate slice: structural/pre-persist and effective-definition validation.
	ValidateStructuralFactoryDefinition(context.Context, ValidateStructuralFactoryDefinitionRequest) (ValidateStructuralFactoryDefinitionResult, error)
	ValidateEffectiveFactoryDefinition(context.Context, ValidateEffectiveFactoryDefinitionRequest) (ValidateEffectiveFactoryDefinitionResult, error)
}

// ListNamedFactoriesRequest selects one Factory definition root for catalog listing.
type ListNamedFactoriesRequest struct {
	RootDir string
}

// ListNamedFactoriesResult carries detached catalog entries peers can consume
// without importing catalog storage implementation types.
type ListNamedFactoriesResult struct {
	Entries []NamedFactoryListEntry
}

// GetNamedFactoryRequest identifies one named Factory under a single root.
type GetNamedFactoryRequest struct {
	RootDir string
	Name    string
}

// GetNamedFactoryResult carries identity facts for one catalog entry.
type GetNamedFactoryResult struct {
	Entry NamedFactoryListEntry
}

// ResolveNamedFactoryRequest resolves one named Factory across project-local
// and global catalog roots using Definitions precedence policy.
type ResolveNamedFactoryRequest struct {
	ProjectRoot string
	GlobalRoot  string
	Name        string
}

// ResolveNamedFactoryResult carries the detached cross-root resolution facts.
type ResolveNamedFactoryResult struct {
	Resolution NamedFactoryResolution
}

// DeleteNamedFactoryRequest identifies one named Factory to remove from a root.
type DeleteNamedFactoryRequest struct {
	RootDir string
	Name    string
}

// DeleteNamedFactoryResult confirms the deleted Factory identity.
type DeleteNamedFactoryResult struct {
	Name       string
	FactoryDir string
}

// GetCurrentFactoryPointerRequest selects the root whose current pointer to read.
type GetCurrentFactoryPointerRequest struct {
	RootDir string
}

// GetCurrentFactoryPointerResult carries the current named-Factory identity.
type GetCurrentFactoryPointerResult struct {
	Name       string
	FactoryDir string
}

// SetCurrentFactoryPointerRequest updates the current pointer under one root.
type SetCurrentFactoryPointerRequest struct {
	RootDir string
	Name    string
}

// SetCurrentFactoryPointerResult confirms the written current-pointer identity.
type SetCurrentFactoryPointerResult struct {
	Name string
}

// ErrMalformedFactoryLayoutPayload reports that authored layout bytes could
// not be prepared as one Factory aggregate.
var ErrMalformedFactoryLayoutPayload = errors.New("malformed factory layout payload")

// ErrAtomicFactoryWriteFailed reports that create/replace did not commit the
// authored Factory aggregate.
var ErrAtomicFactoryWriteFailed = errors.New("atomic factory write failed")

// AtomicFactoryWriteFailure carries failed-write preservation facts without
// exposing peer storage or restore-callback types.
type AtomicFactoryWriteFailure struct {
	Name              string
	FactoryDir        string
	PreviousPreserved bool
	Cause             error
}

func (e *AtomicFactoryWriteFailure) Error() string {
	if e == nil {
		return ErrAtomicFactoryWriteFailed.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("%v: %v", ErrAtomicFactoryWriteFailed, e.Cause)
	}
	if e.PreviousPreserved {
		return fmt.Sprintf(
			"%v: previous layout preserved for %q",
			ErrAtomicFactoryWriteFailed,
			e.Name,
		)
	}
	return ErrAtomicFactoryWriteFailed.Error()
}

func (e *AtomicFactoryWriteFailure) Unwrap() error {
	if e != nil && e.Cause != nil {
		return e.Cause
	}
	return ErrAtomicFactoryWriteFailed
}

func (e *AtomicFactoryWriteFailure) Is(target error) bool {
	return target == ErrAtomicFactoryWriteFailed
}

// PrepareFactoryLayoutRequest carries one authored Factory payload for parse
// and prepare. Callers do not supply filesystem effects or mapping codecs.
type PrepareFactoryLayoutRequest struct {
	Name    string
	Payload []byte
}

// PrepareFactoryLayoutResult carries the Definitions-owned prepared aggregate.
type PrepareFactoryLayoutResult struct {
	Prepared PreparedFactoryLayoutPayload
}

// FlattenFactoryLayoutRequest selects one on-disk or logical Factory path to
// render into canonical authored bytes.
type FlattenFactoryLayoutRequest struct {
	Path string
}

// FlattenFactoryLayoutResult carries detached canonical Factory bytes.
type FlattenFactoryLayoutResult struct {
	Canonical []byte
}

// ExpandFactoryLayoutRequest selects one flattened or split Factory path to
// expand into the authored layout aggregate.
type ExpandFactoryLayoutRequest struct {
	Path string
}

// ExpandFactoryLayoutResult carries portable expand success facts.
type ExpandFactoryLayoutResult struct {
	FactoryDir string
	Report     LayoutExpansionReport
}

// CreateNamedFactoryRequest creates one named Factory from a prepared aggregate.
type CreateNamedFactoryRequest struct {
	RootDir  string
	Name     string
	Prepared PreparedFactoryLayoutPayload
}

// CreateNamedFactoryResult confirms the created Factory identity.
type CreateNamedFactoryResult struct {
	Name       string
	FactoryDir string
}

// ReplaceNamedFactoryRequest replaces one named Factory with a prepared aggregate.
type ReplaceNamedFactoryRequest struct {
	RootDir  string
	Name     string
	Prepared PreparedFactoryLayoutPayload
}

// ReplaceNamedFactoryResult confirms the replaced Factory identity.
type ReplaceNamedFactoryResult struct {
	Name       string
	FactoryDir string
}

// ErrInvalidAuthoredFactorySource reports that authored/canonical bytes could
// not be compiled into one normalized effective source.
var ErrInvalidAuthoredFactorySource = errors.New("invalid authored factory source")

// ErrUnresolvedDefinitionReference reports that compile could not resolve one
// or more definition references in the authored source.
var ErrUnresolvedDefinitionReference = errors.New("unresolved definition reference")

// CompileEffectiveFactorySourceRequest carries authored/canonical Factory
// source for compile/load-effective. Callers do not supply loading
// implementation types or session/runtime lifecycle handles.
type CompileEffectiveFactorySourceRequest struct {
	Canonical  []byte
	FactoryDir string
}

// CompileEffectiveFactorySourceResult carries one Definitions-owned normalized
// effective-source value peers can consume without importing loading types.
type CompileEffectiveFactorySourceResult struct {
	Effective EffectiveFactorySource
}

// EffectiveFactorySource is the Definitions-owned effective-source value
// equivalent to a detached LoadedFactorySource identity/facts projection.
type EffectiveFactorySource struct {
	FactoryDir      string
	RuntimeBaseDir  string
	ContentIdentity string
}

// ErrInvalidFactoryDefinitionPayload reports that validation input could not
// be interpreted as one Factory definition aggregate.
var ErrInvalidFactoryDefinitionPayload = errors.New("invalid factory definition payload")

// ErrFactoryDefinitionValidationFailed reports that structural or effective
// validation produced blocking findings.
var ErrFactoryDefinitionValidationFailed = errors.New("factory definition validation failed")

// FactoryDefinitionValidationFailure carries typed validation findings without
// Petri vocabulary or Runtime/peer storage implementation types.
type FactoryDefinitionValidationFailure struct {
	Validation ValidationResult
	Cause      error
}

func (e *FactoryDefinitionValidationFailure) Error() string {
	if e == nil {
		return ErrFactoryDefinitionValidationFailed.Error()
	}
	if e.Cause != nil {
		return fmt.Sprintf("%v: %v", ErrFactoryDefinitionValidationFailed, e.Cause)
	}
	if e.Validation.HasTargets() {
		errorCount := 0
		for _, target := range e.Validation.Targets {
			if target.Severity == ValidationSeverityError {
				errorCount++
			}
		}
		if errorCount > 0 {
			return fmt.Sprintf(
				"%v: %d error findings",
				ErrFactoryDefinitionValidationFailed,
				errorCount,
			)
		}
	}
	return ErrFactoryDefinitionValidationFailed.Error()
}

func (e *FactoryDefinitionValidationFailure) Unwrap() error {
	if e != nil && e.Cause != nil {
		return e.Cause
	}
	return ErrFactoryDefinitionValidationFailed
}

func (e *FactoryDefinitionValidationFailure) Is(target error) bool {
	return target == ErrFactoryDefinitionValidationFailed
}

// ValidateStructuralFactoryDefinitionRequest carries authored/canonical Factory
// bytes for structural or pre-persist validation. Callers do not supply
// validator collaborators, filesystem effects, or Runtime implementation types.
type ValidateStructuralFactoryDefinitionRequest struct {
	Canonical []byte
	Profile   ValidationProfile
}

// ValidateStructuralFactoryDefinitionResult carries a success-shaped
// ValidationResult with no error findings for valid definitions.
type ValidateStructuralFactoryDefinitionResult struct {
	Validation ValidationResult
}

// ValidateEffectiveFactoryDefinitionRequest carries compiled/effective Factory
// identity facts for effective-definition validation.
type ValidateEffectiveFactoryDefinitionRequest struct {
	Canonical []byte
	Effective EffectiveFactorySource
}

// ValidateEffectiveFactoryDefinitionResult carries a success-shaped
// ValidationResult with no error findings for valid effective definitions.
type ValidateEffectiveFactoryDefinitionResult struct {
	Validation ValidationResult
}

// SessionHost is the Factory Definitions-owned port for session-scoped
// persistence and activation behavior used while composing Service.
type SessionHost interface {
	PersistRootDir() string
	WorkstationLoader() WorkstationLoader
	CurrentRuntimeConfig() LoadedFactorySource
	WorkflowID() string
	RequireSession(string) (*DefinitionSession, error)
	SessionRuntimeConfig(string) (LoadedFactorySource, error)
	SessionFactoryPersistRoot(*DefinitionSession) string
	ValidateEditableFactorySnapshot(context.Context, *FactorySnapshot) error
	GetCurrentFactorySnapshotForSession(context.Context, string) (*FactorySnapshot, error)
	WithActivationLock(func() error) error
	RequireIdleRuntimeForSession(context.Context, string) error
	ActivateSessionEditableFactory(context.Context, *DefinitionSession, string, string, string, string, string) error
	ReplaceFactoryLayoutAtDir(string, *PreparedFactoryLayoutPayload) (*FactorySplitLayoutReplaceResult, error)
	SaveNow() time.Time
	RunSessionID() string
	SessionForActivation(string) *DefinitionSession
	NamedFactoryActivationPaths(*DefinitionSession) (string, string)
	RequireIdleBeforeNamedFactoryActivation(context.Context, string, *DefinitionSession) error
	SwapPersistedNamedFactoryRuntime(context.Context, string, *DefinitionSession, string, string, string, string) error
	AttachFactoryDefinitions(Service) Service
}
