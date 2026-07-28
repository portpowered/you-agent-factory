package runtimeopening

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

type recordingFactoryDefinitionInstaller struct {
	attached factorydefinitions.Service
}

type bindingDefinitionsServiceStub struct {
	factorydefinitions.UnimplementedService
}

func (bindingDefinitionsServiceStub) ActivateNamedFactory(context.Context, string) error {
	return nil
}

func (bindingDefinitionsServiceStub) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (bindingDefinitionsServiceStub) GetCurrentNamedFactory(context.Context) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (bindingDefinitionsServiceStub) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (bindingDefinitionsServiceStub) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (r *recordingFactoryDefinitionInstaller) AttachFactoryDefinitionService(
	service factorydefinitions.Service,
) factorydefinitions.Service {
	r.attached = service
	return service
}

func TestAttachFactoryDefinitionServiceToRuntimeInstallsDefinitions(t *testing.T) {
	t.Parallel()

	installer := &recordingFactoryDefinitionInstaller{}
	owner := bindingDefinitionsServiceStub{}
	if err := attachFactoryDefinitionServiceToRuntime(installer, owner); err != nil {
		t.Fatalf("attachFactoryDefinitionServiceToRuntime() error = %v", err)
	}
	if installer.attached == nil {
		t.Fatal("AttachFactoryDefinitionService() was not called")
	}
}

func TestAttachFactoryDefinitionServiceToRuntimeRejectsMissingBinding(t *testing.T) {
	t.Parallel()

	err := attachFactoryDefinitionServiceToRuntime(struct{}{}, bindingDefinitionsServiceStub{})
	if err == nil || err.Error() != "construct runtime scope: session runtime does not accept Factory Definitions binding" {
		t.Fatalf("attachFactoryDefinitionServiceToRuntime() error = %v, want missing binding", err)
	}
}

func TestAttachFactoryDefinitionServiceToRuntimeRejectsNilDefinitions(t *testing.T) {
	t.Parallel()

	err := attachFactoryDefinitionServiceToRuntime(&recordingFactoryDefinitionInstaller{}, nil)
	if err == nil || err.Error() != "construct runtime scope: Factory Definitions factory returned nil service" {
		t.Fatalf("attachFactoryDefinitionServiceToRuntime() error = %v, want nil service", err)
	}
}
