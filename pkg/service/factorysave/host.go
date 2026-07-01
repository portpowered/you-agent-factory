package factorysave

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// Host exposes session registry, runtime activation, and readback seams owned by pkg/service.
type Host interface {
	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
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
	CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error)
	SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error)
	SerializeNamedFactoryUpsertResponse(
		name factoryapi.FactoryName,
		runtimeCfg *factoryconfig.LoadedFactoryConfig,
	) (factoryapi.Factory, error)
	RequireFreshEditableFactoryVersionAtRoot(
		rootDir string,
		name factoryapi.FactoryName,
		baseVersion *factoryapi.HybridLogicalTimestamp,
	) error
	NextEditableFactoryVersion(
		current *factoryapi.HybridLogicalTimestamp,
		now time.Time,
	) factoryapi.HybridLogicalTimestamp
	PreparePersistedFactoryPayload(
		segment string,
		factory factoryapi.Factory,
		version factoryapi.HybridLogicalTimestamp,
	) (*factoryconfig.PreparedFactoryLayoutPayload, error)
}
