package factorydefinition_test

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinition "github.com/portpowered/infinite-you/pkg/services/factory_definitions/definition"
)

type validationDefinitionHost struct {
	workstationLoader factorydefinitions.WorkstationLoader
}

func (validationDefinitionHost) PersistRootDir() string { return "" }

func (h validationDefinitionHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	return h.workstationLoader
}

func (validationDefinitionHost) CurrentRuntimeConfig() loadedFactorySource {
	return nil
}

func (validationDefinitionHost) WorkflowID() string { return "" }

func (validationDefinitionHost) RequireSession(string) (*factorydefinitions.DefinitionSession, error) {
	return nil, nil
}

func (validationDefinitionHost) SessionRuntimeConfig(string) (loadedFactorySource, error) {
	return nil, nil
}

func (validationDefinitionHost) SessionFactoryPersistRoot(*factorydefinitions.DefinitionSession) string {
	return ""
}

func (h validationDefinitionHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	return validateEditableFactorySnapshotForTest(ctx, snapshot, h.WorkstationLoader())
}

func (validationDefinitionHost) GetCurrentFactorySnapshotForSession(context.Context, string) (*factorydefinitions.FactorySnapshot, error) {
	return mustFactorySnapshot(factoryapi.Factory{}), nil
}

func (validationDefinitionHost) WithActivationLock(fn func() error) error { return fn() }

func (validationDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (validationDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorydefinitions.DefinitionSession, string, string, string, string, string) error {
	return nil
}

func (validationDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (validationDefinitionHost) SaveNow() time.Time { return time.Time{} }

func (validationDefinitionHost) RunSessionID() string { return "" }

func (validationDefinitionHost) SessionForActivation(string) *factorydefinitions.DefinitionSession {
	return nil
}

func (validationDefinitionHost) NamedFactoryActivationPaths(*factorydefinitions.DefinitionSession) (string, string) {
	return "", ""
}

func (validationDefinitionHost) RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorydefinitions.DefinitionSession) error {
	return nil
}

func (validationDefinitionHost) SwapPersistedNamedFactoryRuntime(context.Context, string, *factorydefinitions.DefinitionSession, string, string, string, string) error {
	return nil
}

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factorydefinitions.WorkstationLoader) error {
	snapshot := mustFactorySnapshot(submitted)
	return factorydefinition.New(validationDefinitionHost{
		workstationLoader: workstationLoader,
	}).ValidateEditableFactoryTopology(context.Background(), snapshot)
}

func validateUpsertNamedFactoryRequest(
	request factoryapi.Factory,
	workstationLoader factorydefinitions.WorkstationLoader,
) error {
	snapshot := mustFactorySnapshot(request)
	return factorydefinition.New(validationDefinitionHost{
		workstationLoader: workstationLoader,
	}).ValidateUpsertNamedFactoryRequest(context.Background(), string(request.Name), snapshot)
}
