package factorydefinitions

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	// Catalog slice: effective discovery, list, get/resolve, delete, and
	// current-pointer read/write.
	ListEffectiveFactories(context.Context, ListEffectiveFactoriesRequest) (ListEffectiveFactoriesResult, error)
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

	// Snapshot slice: capture, import/prepare, and materialize of detached
	// Factory snapshots and bundled assets.
	CaptureFactorySnapshot(context.Context, CaptureFactorySnapshotRequest) (CaptureFactorySnapshotResult, error)
	PrepareFactorySnapshotImport(context.Context, PrepareFactorySnapshotImportRequest) (PrepareFactorySnapshotImportResult, error)
	MaterializeFactorySnapshot(context.Context, MaterializeFactorySnapshotRequest) (MaterializeFactorySnapshotResult, error)

	// Distribute slice: built-in package catalog listing, packaged installation,
	// and scaffold creation. Install and scaffold return the same Definition
	// aggregate identity/facts shape.
	ListBuiltInPackagedFactories(context.Context, ListBuiltInPackagedFactoriesRequest) (ListBuiltInPackagedFactoriesResult, error)
	ResolveBuiltInPackagedFactory(context.Context, ResolveBuiltInPackagedFactoryRequest) (ResolveBuiltInPackagedFactoryResult, error)
	InstallPackagedFactory(context.Context, InstallPackagedFactoryRequest) (InstallPackagedFactoryResult, error)
	CreateFactoryScaffold(context.Context, CreateFactoryScaffoldRequest) (CreateFactoryScaffoldResult, error)
}

// ListEffectiveFactoriesRequest selects the project-local and global roots
// merged with the published packaged Factory catalog.
type ListEffectiveFactoriesRequest struct {
	ProjectRoot string
	GlobalRoot  string
}

// ListEffectiveFactoriesResult carries the detached, precedence-resolved
// Factory catalog used by read-only transport projections.
type ListEffectiveFactoriesResult struct {
	Entries     []EffectiveFactoryCatalogEntry
	Diagnostics []EffectiveFactoryCatalogDiagnostic
}

// EffectiveFactoryCatalogSource identifies one precedence tier without
// exposing filesystem paths or packaged payload details.
type EffectiveFactoryCatalogSource string

const (
	EffectiveFactoryCatalogSourceProjectLocal EffectiveFactoryCatalogSource = "project-local"
	EffectiveFactoryCatalogSourceGlobal       EffectiveFactoryCatalogSource = "global"
	EffectiveFactoryCatalogSourcePackaged     EffectiveFactoryCatalogSource = "packaged"
)

// EffectiveFactoryCatalogDiagnosticCode classifies one isolated candidate
// failure without retaining the underlying error or definition payload.
type EffectiveFactoryCatalogDiagnosticCode string

const (
	EffectiveFactoryCatalogDiagnosticInvalidName EffectiveFactoryCatalogDiagnosticCode = "invalid-name"
	EffectiveFactoryCatalogDiagnosticUnreadable  EffectiveFactoryCatalogDiagnosticCode = "unreadable"
	EffectiveFactoryCatalogDiagnosticMalformed   EffectiveFactoryCatalogDiagnosticCode = "malformed"
)

// EffectiveFactoryCatalogDiagnostic is a deterministic, sensitive-safe
// description of one candidate omitted from the effective catalog. Name is
// empty when the candidate name cannot be safely canonicalized.
type EffectiveFactoryCatalogDiagnostic struct {
	Code    EffectiveFactoryCatalogDiagnosticCode
	Source  EffectiveFactoryCatalogSource
	Name    string
	Message string
}

// EffectiveFactoryCatalogEntry is one normalized selectable Factory. Location
// is nil for packaged definitions that have not been materialized.
type EffectiveFactoryCatalogEntry struct {
	Name                string
	Location            *string
	Definition          *FactoryConfig
	InvocationSignature *InvocationSignatureConfig
}

// EffectiveFactoryCatalogCandidate is one discovered definition payload.
// Sources return detached payload bytes and omit Location for packaged entries.
type EffectiveFactoryCatalogCandidate struct {
	Name      string
	Location  *string
	Canonical []byte
	Failure   EffectiveFactoryCatalogDiagnosticCode
}

// EffectiveFactoryCatalogDiscovery carries the exact read-only source
// operations used by effective discovery.
type EffectiveFactoryCatalogDiscovery struct {
	ListRoot     func(context.Context, string) ([]EffectiveFactoryCatalogCandidate, error)
	ListPackaged func(context.Context) ([]EffectiveFactoryCatalogCandidate, error)
}

// EffectiveFactoryRootListing reads one persisted named-Factory root.
type EffectiveFactoryRootListing func(string) ([]NamedFactoryListEntry, error)

// EffectiveFactoryCandidateRead reads one already-discovered canonical Factory
// definition payload.
type EffectiveFactoryCandidateRead func(string) ([]byte, error)

// EffectiveFactoryDefinitionNormalizer converts one candidate payload into the
// normalized Factory definition consumed by transport projections.
type EffectiveFactoryDefinitionNormalizer func(
	context.Context,
	EffectiveFactoryCatalogCandidate,
) (*FactoryConfig, error)

// EffectiveFactoryCatalogOperation owns precedence, shadowing, stable ordering,
// and detached results for effective Factory discovery.
type EffectiveFactoryCatalogOperation func(
	context.Context,
	ListEffectiveFactoriesRequest,
) (ListEffectiveFactoriesResult, error)

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

// ErrInvalidFactorySnapshotPayload reports that snapshot bytes were not a
// detached Factory object peers can import or capture.
var ErrInvalidFactorySnapshotPayload = errors.New("invalid factory snapshot payload")

// ErrUnsafeFactorySnapshotMaterialize reports that materialize inputs were
// incomplete or unsafe (for example missing target identity or path escape).
var ErrUnsafeFactorySnapshotMaterialize = errors.New("unsafe or incomplete factory snapshot materialize")

// CaptureFactorySnapshotRequest selects one authored/effective Factory source
// for detached snapshot capture. Callers do not supply snapshotcapture or
// Recordings/Runtime implementation types.
type CaptureFactorySnapshotRequest struct {
	FactoryDir string
	Canonical  []byte
	Name       string
}

// CaptureFactorySnapshotResult carries one detached FactorySnapshot peers can
// consume without importing snapshotcapture implementation types.
type CaptureFactorySnapshotResult struct {
	Snapshot *FactorySnapshot
}

// PrepareFactorySnapshotImportRequest carries raw snapshot payload bytes for
// import/prepare. Callers do not supply boundary codecs as request fields.
type PrepareFactorySnapshotImportRequest struct {
	Payload []byte
}

// PrepareFactorySnapshotImportResult carries Definitions-owned portable import
// success facts and the detached snapshot value.
type PrepareFactorySnapshotImportResult struct {
	Snapshot *FactorySnapshot
	Name     string
	Portable PortableFactorySnapshotFacts
}

// MaterializeFactorySnapshotRequest materializes one detached snapshot and its
// bundled assets under a target directory.
type MaterializeFactorySnapshotRequest struct {
	TargetDir string
	Snapshot  *FactorySnapshot
}

// MaterializeFactorySnapshotResult carries Definitions-owned portable
// materialize success facts without peer storage types.
type MaterializeFactorySnapshotResult struct {
	TargetDir string
	Portable  PortableFactorySnapshotFacts
}

// PortableFactorySnapshotFacts are detached portability identity/asset facts
// peers can consume without importing bundled-asset implementation types.
type PortableFactorySnapshotFacts struct {
	FactoryDir string
	Assets     []PortableSnapshotAssetFact
}

// PortableSnapshotAssetFact identifies one materialized or imported portable
// bundled asset relative to the Factory root.
type PortableSnapshotAssetFact struct {
	TargetPath string
}

// ErrUnknownPackagedFactoryIdentity reports that a packaged Factory name is
// not present in the built-in package catalog.
var ErrUnknownPackagedFactoryIdentity = errors.New("unknown packaged factory identity")

// ErrFactoryDistributeFailed reports that packaged installation or scaffold
// creation did not produce a Factory Definition aggregate.
var ErrFactoryDistributeFailed = errors.New("factory distribute failed")

// DistributedFactoryDefinitionFacts are the shared Definition aggregate
// identity/facts returned by install and scaffold distribute paths.
type DistributedFactoryDefinitionFacts struct {
	Name       string
	FactoryDir string
}

// ListBuiltInPackagedFactoriesRequest lists built-in packaged Factory identities
// published with the executable. Callers do not supply catalog storage types.
type ListBuiltInPackagedFactoriesRequest struct{}

// ListBuiltInPackagedFactoriesResult carries detached built-in package catalog
// entries peers can consume without importing packagedinstallation types.
type ListBuiltInPackagedFactoriesResult struct {
	Entries []BuiltInPackagedFactoryEntry
}

// BuiltInPackagedFactoryEntry identifies one built-in packaged Factory.
type BuiltInPackagedFactoryEntry struct {
	Name    string
	Project string
	Formats []PackagedFactoryFormat
}

// ResolveBuiltInPackagedFactoryRequest selects one built-in packaged Factory
// by its public manifest name.
type ResolveBuiltInPackagedFactoryRequest struct {
	Name string
}

// ResolveBuiltInPackagedFactoryResult carries a detached definition and the
// representation formats published for that same manifest entry.
type ResolveBuiltInPackagedFactoryResult struct {
	Definition PackagedDefinition
	Formats    []PackagedFactoryFormat
}

// UnknownPackagedFactoryError reports a missing public name together with the
// stable public inventory. It intentionally omits embedded artifact locators.
type UnknownPackagedFactoryError struct {
	Name      string
	Available []string
}

func (e *UnknownPackagedFactoryError) Error() string {
	if e == nil {
		return ErrUnknownPackagedFactoryIdentity.Error()
	}
	return fmt.Sprintf(
		"%v %q; available packaged factories: %s",
		ErrUnknownPackagedFactoryIdentity,
		e.Name,
		strings.Join(e.Available, ", "),
	)
}

func (e *UnknownPackagedFactoryError) Is(target error) bool {
	return target == ErrUnknownPackagedFactoryIdentity
}

// PackagedFactoryCatalogOperations are the Definitions-owned, read-only
// catalog operations shared by bootstrap and customer-facing selection.
type PackagedFactoryCatalogOperations struct {
	List    func(context.Context, ListBuiltInPackagedFactoriesRequest) (ListBuiltInPackagedFactoriesResult, error)
	Resolve func(context.Context, ResolveBuiltInPackagedFactoryRequest) (ResolveBuiltInPackagedFactoryResult, error)
}

func (operations PackagedFactoryCatalogOperations) ListBuiltInPackagedFactories(
	ctx context.Context,
	request ListBuiltInPackagedFactoriesRequest,
) (ListBuiltInPackagedFactoriesResult, error) {
	if operations.List == nil {
		return ListBuiltInPackagedFactoriesResult{}, fmt.Errorf("packaged factory catalog collaborator is required")
	}
	return operations.List(ctx, request)
}

func (operations PackagedFactoryCatalogOperations) ResolveBuiltInPackagedFactory(
	ctx context.Context,
	request ResolveBuiltInPackagedFactoryRequest,
) (ResolveBuiltInPackagedFactoryResult, error) {
	if operations.Resolve == nil {
		return ResolveBuiltInPackagedFactoryResult{}, ErrUnknownPackagedFactoryIdentity
	}
	return operations.Resolve(ctx, request)
}

// InstallPackagedFactoryRequest installs one built-in packaged Factory by
// identity under a named-Factory root. Callers do not supply PackagedDefinition
// payload bytes or installer collaborators as request fields.
type InstallPackagedFactoryRequest struct {
	RootDir  string
	Name     string
	Format   PackagedFactoryFormat
	Replace  bool
	Scaffold CreateFactoryScaffoldRequest
}

// InstallPackagedFactoryResult carries Definitions-owned distribute success
// facts for one installed packaged Factory.
type InstallPackagedFactoryResult struct {
	Definition DistributedFactoryDefinitionFacts
	Outcome    PackagedFactoryInstallOutcome
	Format     PackagedFactoryFormat
}

// PackagedFactoryInstallationOperations are the Definitions-owned write
// operations used after catalog selection has returned a detached definition.
type PackagedFactoryInstallationOperations struct {
	Install func(
		context.Context,
		PackagedFactoryInstallParams,
	) (PackagedFactoryInstallResult, error)
}

func (operations PackagedFactoryInstallationOperations) InstallPackagedFactory(
	ctx context.Context,
	params PackagedFactoryInstallParams,
) (PackagedFactoryInstallResult, error) {
	if operations.Install == nil {
		return PackagedFactoryInstallResult{}, fmt.Errorf(
			"packaged Factory installation collaborator is required",
		)
	}
	return operations.Install(ctx, params)
}

// CreateFactoryScaffoldRequest creates one Factory scaffold under a target
// directory. Callers do not supply filesystem effects or output streams as
// part of the cross-service request shape.
type CreateFactoryScaffoldRequest struct {
	TargetDir string
	Type      string
	Executor  string
}

// CreateFactoryScaffoldResult carries Definitions-owned distribute success
// facts for one scaffolded Factory aggregate.
type CreateFactoryScaffoldResult struct {
	Definition   DistributedFactoryDefinitionFacts
	ScaffoldType string
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
	ReplaceFactoryLayoutAtDir(string, *PreparedFactoryLayoutPayload) (*FactorySplitLayoutReplaceResult, error)
	AttachFactoryDefinitions(Service) Service
}
