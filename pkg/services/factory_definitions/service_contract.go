package factorydefinitions

import (
	"context"
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
