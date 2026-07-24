package factorydefinitions_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// fakeDefinitionsPeer is a peer-owned stand-in that depends only on the
// Factory Definitions root package. It proves cross-service consumers can
// satisfy the singular root Service without importing Definitions
// implementation subpackages.
type fakeDefinitionsPeer struct{}

func (fakeDefinitionsPeer) ActivateNamedFactory(context.Context, string) error {
	return nil
}

func (fakeDefinitionsPeer) Save(
	context.Context,
	string,
	factorydefinitions.SaveMode,
	factorydefinitions.EditableFactory,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, nil
}

func (fakeDefinitionsPeer) GetCurrentNamedFactory(
	context.Context,
) (*factorydefinitions.FactorySnapshot, error) {
	return nil, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) GetCurrentFactoryForSession(
	context.Context,
	string,
) (factorydefinitions.EditableFactory, error) {
	return factorydefinitions.EditableFactory{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) CurrentFactoryDefinitionVersionAtRoot(
	string,
	string,
) (factorydefinitions.FactoryVersion, error) {
	return factorydefinitions.FactoryVersion{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func TestRootService_FakePeerReadPath_TypedNotFound(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}
	snapshot, err := service.GetCurrentNamedFactory(context.Background())
	if snapshot != nil {
		t.Fatalf("GetCurrentNamedFactory snapshot = %#v, want nil", snapshot)
	}
	if !errors.Is(err, factorydefinitions.ErrCurrentFactoryNotFound) {
		t.Fatalf(
			"GetCurrentNamedFactory error = %v, want %v",
			err,
			factorydefinitions.ErrCurrentFactoryNotFound,
		)
	}
}
