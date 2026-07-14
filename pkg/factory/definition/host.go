package factorydefinition

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
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

	GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error)
	WithActivationLock(fn func() error) error
	RequireIdleRuntimeForSession(ctx context.Context, sessionID string) error
	ActivateSessionEditableFactory(
		ctx context.Context,
		session *factorysessions.LiveSession,
		sessionID string,
		sessionRootDir string,
		factoryDir string,
		name factoryapi.FactoryName,
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
