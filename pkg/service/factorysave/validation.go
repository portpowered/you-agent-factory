package factorysave

import (
	"context"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessions"
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

func (validationDefinitionHost) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	return factoryapi.Factory{}, nil
}

func (validationDefinitionHost) WithActivationLock(fn func() error) error { return fn() }

func (validationDefinitionHost) RequireIdleRuntimeForSession(context.Context, string) error {
	return nil
}

func (validationDefinitionHost) ActivateSessionEditableFactory(context.Context, *factorysessions.LiveSession, string, string, string, factoryapi.FactoryName, string) error {
	return nil
}

func (validationDefinitionHost) ReplaceFactoryLayoutAtDir(string, *factoryconfig.PreparedFactoryLayoutPayload) (*factoryconfig.FactorySplitLayoutReplaceResult, error) {
	return nil, nil
}

func (validationDefinitionHost) SaveNow() time.Time { return time.Time{} }

func validateEditableFactoryTopology(submitted factoryapi.Factory, workstationLoader factoryconfig.WorkstationLoader) error {
	return factorydefinition.New(validationDefinitionHost{
		workstationLoader: workstationLoader,
	}).ValidateEditableFactoryTopology(submitted)
}

func validateUpsertNamedFactoryRequest(
	request factoryapi.Factory,
	workstationLoader factoryconfig.WorkstationLoader,
) error {
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return err
	}
	return validateEditableFactoryTopology(request, workstationLoader)
}
