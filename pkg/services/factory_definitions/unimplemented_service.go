package factorydefinitions

import (
	"context"
	"fmt"
)

// UnimplementedService provides typed CTR-DEF root-slice defaults so concrete
// implementers stay assignable to Service before nested IMP-DEF wiring lands.
// Embed it on owner-local implementers and override only the methods that are
// already connected to collaborators.
type UnimplementedService struct{}

var _ Service = UnimplementedService{}

// ListEffectiveFactories returns a collaborator-required failure until nested
// effective-catalog wiring lands.
func (UnimplementedService) ListEffectiveFactories(
	context.Context,
	ListEffectiveFactoriesRequest,
) (ListEffectiveFactoriesResult, error) {
	return ListEffectiveFactoriesResult{}, fmt.Errorf("effective Factory catalog collaborator is required")
}

// ListNamedFactories returns a collaborator-required failure until nested
// catalog wiring lands.
func (UnimplementedService) ListNamedFactories(
	context.Context,
	ListNamedFactoriesRequest,
) (ListNamedFactoriesResult, error) {
	return ListNamedFactoriesResult{}, fmt.Errorf("named factory catalog collaborator is required")
}

// GetNamedFactory returns ErrNamedFactoryNotFound until nested catalog wiring lands.
func (UnimplementedService) GetNamedFactory(
	context.Context,
	GetNamedFactoryRequest,
) (GetNamedFactoryResult, error) {
	return GetNamedFactoryResult{}, ErrNamedFactoryNotFound
}

// ResolveNamedFactory returns ErrNamedFactoryNotFound until nested catalog wiring lands.
func (UnimplementedService) ResolveNamedFactory(
	context.Context,
	ResolveNamedFactoryRequest,
) (ResolveNamedFactoryResult, error) {
	return ResolveNamedFactoryResult{}, ErrNamedFactoryNotFound
}

// DeleteNamedFactory returns ErrNamedFactoryNotFound until nested catalog wiring lands.
func (UnimplementedService) DeleteNamedFactory(
	context.Context,
	DeleteNamedFactoryRequest,
) (DeleteNamedFactoryResult, error) {
	return DeleteNamedFactoryResult{}, ErrNamedFactoryNotFound
}

// GetCurrentFactoryPointer returns ErrCurrentFactoryNotFound until nested
// current-pointer wiring lands on the owner implementer.
func (UnimplementedService) GetCurrentFactoryPointer(
	context.Context,
	GetCurrentFactoryPointerRequest,
) (GetCurrentFactoryPointerResult, error) {
	return GetCurrentFactoryPointerResult{}, ErrCurrentFactoryNotFound
}

// SetCurrentFactoryPointer returns ErrNamedFactoryNotFound until nested
// current-pointer wiring lands on the owner implementer.
func (UnimplementedService) SetCurrentFactoryPointer(
	context.Context,
	SetCurrentFactoryPointerRequest,
) (SetCurrentFactoryPointerResult, error) {
	return SetCurrentFactoryPointerResult{}, ErrNamedFactoryNotFound
}

// ClearCurrentFactoryPointer returns a collaborator-required failure until
// nested current-pointer wiring lands on the owner implementer.
func (UnimplementedService) ClearCurrentFactoryPointer(
	context.Context,
	ClearCurrentFactoryPointerRequest,
) (ClearCurrentFactoryPointerResult, error) {
	return ClearCurrentFactoryPointerResult{}, fmt.Errorf("current Factory pointer collaborator is required")
}

// PrepareFactoryLayout returns ErrMalformedFactoryLayoutPayload until nested
// layout wiring lands.
func (UnimplementedService) PrepareFactoryLayout(
	context.Context,
	PrepareFactoryLayoutRequest,
) (PrepareFactoryLayoutResult, error) {
	return PrepareFactoryLayoutResult{}, ErrMalformedFactoryLayoutPayload
}

// FlattenFactoryLayout returns a collaborator-required failure until nested
// layout wiring lands.
func (UnimplementedService) FlattenFactoryLayout(
	context.Context,
	FlattenFactoryLayoutRequest,
) (FlattenFactoryLayoutResult, error) {
	return FlattenFactoryLayoutResult{}, fmt.Errorf("factory layout collaborator is required")
}

// ExpandFactoryLayout returns a collaborator-required failure until nested
// layout wiring lands.
func (UnimplementedService) ExpandFactoryLayout(
	context.Context,
	ExpandFactoryLayoutRequest,
) (ExpandFactoryLayoutResult, error) {
	return ExpandFactoryLayoutResult{}, fmt.Errorf("factory layout collaborator is required")
}

// CreateNamedFactory returns AtomicFactoryWriteFailure until nested layout
// wiring lands.
func (UnimplementedService) CreateNamedFactory(
	context.Context,
	CreateNamedFactoryRequest,
) (CreateNamedFactoryResult, error) {
	return CreateNamedFactoryResult{}, &AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

// ReplaceNamedFactory returns AtomicFactoryWriteFailure until nested layout
// wiring lands.
func (UnimplementedService) ReplaceNamedFactory(
	context.Context,
	ReplaceNamedFactoryRequest,
) (ReplaceNamedFactoryResult, error) {
	return ReplaceNamedFactoryResult{}, &AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

// CompileEffectiveFactorySource returns ErrInvalidAuthoredFactorySource until
// nested loading wiring lands.
func (UnimplementedService) CompileEffectiveFactorySource(
	context.Context,
	CompileEffectiveFactorySourceRequest,
) (CompileEffectiveFactorySourceResult, error) {
	return CompileEffectiveFactorySourceResult{}, ErrInvalidAuthoredFactorySource
}

// ValidateStructuralFactoryDefinition returns ErrInvalidFactoryDefinitionPayload
// until nested validator wiring lands.
func (UnimplementedService) ValidateStructuralFactoryDefinition(
	context.Context,
	ValidateStructuralFactoryDefinitionRequest,
) (ValidateStructuralFactoryDefinitionResult, error) {
	return ValidateStructuralFactoryDefinitionResult{}, ErrInvalidFactoryDefinitionPayload
}

// ValidateEffectiveFactoryDefinition returns ErrInvalidFactoryDefinitionPayload
// until nested validator wiring lands.
func (UnimplementedService) ValidateEffectiveFactoryDefinition(
	context.Context,
	ValidateEffectiveFactoryDefinitionRequest,
) (ValidateEffectiveFactoryDefinitionResult, error) {
	return ValidateEffectiveFactoryDefinitionResult{}, ErrInvalidFactoryDefinitionPayload
}

// CaptureFactorySnapshot returns ErrInvalidFactorySnapshotPayload until nested
// snapshotcapture wiring lands.
func (UnimplementedService) CaptureFactorySnapshot(
	context.Context,
	CaptureFactorySnapshotRequest,
) (CaptureFactorySnapshotResult, error) {
	return CaptureFactorySnapshotResult{}, ErrInvalidFactorySnapshotPayload
}

// PrepareFactorySnapshotImport returns ErrInvalidFactorySnapshotPayload until
// nested snapshotcapture wiring lands.
func (UnimplementedService) PrepareFactorySnapshotImport(
	context.Context,
	PrepareFactorySnapshotImportRequest,
) (PrepareFactorySnapshotImportResult, error) {
	return PrepareFactorySnapshotImportResult{}, ErrInvalidFactorySnapshotPayload
}

// MaterializeFactorySnapshot returns ErrUnsafeFactorySnapshotMaterialize until
// nested portable materialize wiring lands.
func (UnimplementedService) MaterializeFactorySnapshot(
	context.Context,
	MaterializeFactorySnapshotRequest,
) (MaterializeFactorySnapshotResult, error) {
	return MaterializeFactorySnapshotResult{}, ErrUnsafeFactorySnapshotMaterialize
}

// ListBuiltInPackagedFactories returns a collaborator-required failure until
// nested packaged-catalog wiring lands.
func (UnimplementedService) ListBuiltInPackagedFactories(
	context.Context,
	ListBuiltInPackagedFactoriesRequest,
) (ListBuiltInPackagedFactoriesResult, error) {
	return ListBuiltInPackagedFactoriesResult{}, fmt.Errorf("packaged factory catalog collaborator is required")
}

// ResolveBuiltInPackagedFactory returns ErrUnknownPackagedFactoryIdentity until
// nested packaged-catalog wiring lands.
func (UnimplementedService) ResolveBuiltInPackagedFactory(
	context.Context,
	ResolveBuiltInPackagedFactoryRequest,
) (ResolveBuiltInPackagedFactoryResult, error) {
	return ResolveBuiltInPackagedFactoryResult{}, ErrUnknownPackagedFactoryIdentity
}

// InstallPackagedFactory returns ErrUnknownPackagedFactoryIdentity until nested
// packaged installation wiring lands.
func (UnimplementedService) InstallPackagedFactory(
	context.Context,
	InstallPackagedFactoryRequest,
) (InstallPackagedFactoryResult, error) {
	return InstallPackagedFactoryResult{}, ErrUnknownPackagedFactoryIdentity
}

// CreateFactoryScaffold returns ErrFactoryDistributeFailed until nested scaffold
// wiring lands.
func (UnimplementedService) CreateFactoryScaffold(
	context.Context,
	CreateFactoryScaffoldRequest,
) (CreateFactoryScaffoldResult, error) {
	return CreateFactoryScaffoldResult{}, ErrFactoryDistributeFailed
}

// ResolveInvocationDefinition returns a typed contract failure until the
// Definitions-owned invocation resolver is wired.
func (UnimplementedService) ResolveInvocationDefinition(
	context.Context,
	ResolveInvocationDefinitionRequest,
) (ResolveInvocationDefinitionResult, error) {
	return ResolveInvocationDefinitionResult{}, ErrInvalidInvocationDefinition
}
