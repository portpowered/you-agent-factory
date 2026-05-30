package factorysave

import (
	"context"

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
	ReplaceDefaultFactoryDefinition(sessionRootDir string, payload []byte) (restore func(), err error)
	CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error)
	SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error)
	SerializeNamedFactoryUpsertResponse(
		name factoryapi.FactoryName,
		runtimeCfg *factoryconfig.LoadedFactoryConfig,
	) (factoryapi.Factory, error)
}
