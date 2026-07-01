package factorydefinition

import (
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

// Host supplies session and runtime collaborators required for definition reads.
type Host interface {
	PersistRootDir() string
	WorkstationLoader() factoryconfig.WorkstationLoader
	CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig
	WorkflowID() string

	RequireSession(sessionID string) (*factorysessions.LiveSession, error)
	SessionRuntimeConfig(sessionID string) (*factoryconfig.LoadedFactoryConfig, error)
	SessionFactoryPersistRoot(session *factorysessions.LiveSession) string
}
