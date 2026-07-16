package factorysave

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factory/definition"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/validationentry"
)

type validationDefinitionHost struct {
	workstationLoader factoryconfig.WorkstationLoader
}

func (validationDefinitionHost) PersistRootDir() string { return "" }

func (h validationDefinitionHost) WorkstationLoader() factoryconfig.WorkstationLoader {
	return h.workstationLoader
}

func (validationDefinitionHost) CurrentRuntimeConfig() *factoryconfig.LoadedFactoryConfig {
	return nil
}

func (validationDefinitionHost) WorkflowID() string { return "" }

func (validationDefinitionHost) RequireSession(string) (*factorysessions.LiveSession, error) {
	return nil, nil
}

func (validationDefinitionHost) SessionRuntimeConfig(string) (*factoryconfig.LoadedFactoryConfig, error) {
	return nil, nil
}

func (validationDefinitionHost) SessionFactoryPersistRoot(*factorysessions.LiveSession) string {
	return ""
}

func (h validationDefinitionHost) ValidateEditableFactorySnapshot(snapshot *interfaces.FactorySnapshot) error {
	return validationentry.ValidateEditableFactorySnapshot(snapshot, h.WorkstationLoader())
}

func (validationDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*interfaces.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (validationDefinitionHost) WithActivationLock(fn func() error) error { return fn() }

func (validationDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (validationDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, string, string) error {
	return nil
}

func (validationDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (validationDefinitionHost) SaveNow() time.Time { return time.Time{} }

func (validationDefinitionHost) RunSessionID() string { return "" }

func (validationDefinitionHost) SessionForActivation(string) *factorysessions.LiveSession {
	return nil
}

func (validationDefinitionHost) NamedFactoryActivationPaths(*factorysessions.LiveSession) (string, string) {
	return "", ""
}

func (validationDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorysessions.LiveSession) error {
	return nil
}

func (validationDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorysessions.LiveSession, string, string, string, string) error {
	return nil
}

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factoryconfig.WorkstationLoader) error {
	snapshot := mustFactorySnapshot(submitted)
	return factorydefinition.New(validationDefinitionHost{
		workstationLoader: workstationLoader,
	}).ValidateEditableFactoryTopology(snapshot)
}

func validateUpsertNamedFactoryRequest(
	request factoryapi.Factory,
	workstationLoader factoryconfig.WorkstationLoader,
) error {
	snapshot := mustFactorySnapshot(request)
	return factorydefinition.New(validationDefinitionHost{
		workstationLoader: workstationLoader,
	}).ValidateUpsertNamedFactoryRequest(string(request.Name), snapshot)
}
