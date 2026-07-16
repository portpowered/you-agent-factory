package factorydefinition

import (
	"context"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorycontracts "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// Host supplies session and runtime collaborators required for definition reads
// and current-factory save orchestration.
type Host interface {
	PersistRootDir() string
	WorkstationLoader() factoryconfig.WorkstationLoader
	CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig
	WorkflowID() string

	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error)
	SessionFactoryPersistRoot(session *factorysessions.LiveSession) string
	ValidateEditableFactorySnapshot(snapshot *factorycontracts.FactorySnapshot) error

	GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*factorycontracts.FactorySnapshot, error)
	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	ActivateSessionEditableFactory(
		ctx context.Context,
		session *factorysessions.LiveSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name string,
		runtimeName string,
	) error
	ReplaceFactoryLayoutAtDir(targetDir string, prepared *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error)
	SaveNow() time.Time

	RunSessionID() string
	SessionForActivation(sessionID string) *factorysessions.LiveSession
	NamedFactoryActivationPaths(session *factorysessions.LiveSession) (persistRoot, folderPath string)
	RequireIdleBeforeNamedFactoryActivation(ctx context.Context, sessionID string, session *factorysessions.LiveSession) error
	SwapPersistedNamedFactoryRuntime(
		ctx context.Context,
		sessionID string,
		session *factorysessions.LiveSession,
		persistRoot string,
		folderPath string,
		factoryDir string,
		name string,
	) error
}
