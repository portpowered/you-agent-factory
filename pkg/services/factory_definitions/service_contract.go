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
