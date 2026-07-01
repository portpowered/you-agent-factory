package factorysave

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
)

var testDefinitionService = factorydefinition.New(stubDefinitionHost{})

type stubDefinitionHost struct{}

func (stubDefinitionHost) PersistRootDir() string { return "" }
func (stubDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	return nil
}
func (stubDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return nil
}
func (stubDefinitionHost) WorkflowID() string { return "" }
func (stubDefinitionHost) RequireSession(string) (*factorysessions.LiveSession, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, nil
}
func (stubDefinitionHost) SessionFactoryPersistRoot(*factorysessions.LiveSession) string {
	return ""
}

func requireFreshEditableFactoryVersion(
	baseVersion *factoryapi.HybridLogicalTimestamp,
	currentVersion factoryapi.HybridLogicalTimestamp,
) error {
	return testDefinitionService.RequireFreshEditableFactoryVersion(baseVersion, currentVersion)
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	return testDefinitionService.NextEditableFactoryVersion(current, now)
}

func preparePersistedFactoryPayload(
	segment string,
	factory factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) (*factoryconfig.PreparedFactoryLayoutPayload, error) {
	return testDefinitionService.PreparePersistedFactoryPayload(segment, factory, version)
}

func prepareEditableFactoryPersistView(
	segment string,
	factory factoryapi.Factory,
) (*configpersist.PreparedFactoryLayoutPayload, error) {
	return testDefinitionService.PrepareEditableFactoryPersistView(segment, factory)
}
