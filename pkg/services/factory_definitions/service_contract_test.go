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
type fakeDefinitionsPeer struct {
	entries []factorydefinitions.NamedFactoryListEntry
}

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

func (p fakeDefinitionsPeer) ListNamedFactories(
	_ context.Context,
	_ factorydefinitions.ListNamedFactoriesRequest,
) (factorydefinitions.ListNamedFactoriesResult, error) {
	entries := append([]factorydefinitions.NamedFactoryListEntry(nil), p.entries...)
	return factorydefinitions.ListNamedFactoriesResult{Entries: entries}, nil
}

func (p fakeDefinitionsPeer) GetNamedFactory(
	_ context.Context,
	request factorydefinitions.GetNamedFactoryRequest,
) (factorydefinitions.GetNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	for _, entry := range p.entries {
		if entry.Name == request.Name {
			return factorydefinitions.GetNamedFactoryResult{Entry: entry}, nil
		}
	}
	return factorydefinitions.GetNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (p fakeDefinitionsPeer) ResolveNamedFactory(
	_ context.Context,
	request factorydefinitions.ResolveNamedFactoryRequest,
) (factorydefinitions.ResolveNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	for _, entry := range p.entries {
		if entry.Name == request.Name {
			return factorydefinitions.ResolveNamedFactoryResult{
				Resolution: factorydefinitions.NamedFactoryResolution{
					Name:               entry.Name,
					FactoryDir:         entry.FactoryDir,
					Source:             factorydefinitions.NamedFactoryResolutionSourceProjectLocal,
					ProjectRoot:        request.ProjectRoot,
					GlobalRoot:         request.GlobalRoot,
					PrecedenceDecision: factorydefinitions.NamedFactoryPrecedenceDecisionNone,
				},
			}, nil
		}
	}
	return factorydefinitions.ResolveNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (fakeDefinitionsPeer) DeleteNamedFactory(
	_ context.Context,
	request factorydefinitions.DeleteNamedFactoryRequest,
) (factorydefinitions.DeleteNamedFactoryResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	return factorydefinitions.DeleteNamedFactoryResult{}, factorydefinitions.ErrNamedFactoryNotFound
}

func (p fakeDefinitionsPeer) GetCurrentFactoryPointer(
	_ context.Context,
	_ factorydefinitions.GetCurrentFactoryPointerRequest,
) (factorydefinitions.GetCurrentFactoryPointerResult, error) {
	for _, entry := range p.entries {
		if entry.Current {
			return factorydefinitions.GetCurrentFactoryPointerResult{
				Name:       entry.Name,
				FactoryDir: entry.FactoryDir,
			}, nil
		}
	}
	return factorydefinitions.GetCurrentFactoryPointerResult{}, factorydefinitions.ErrCurrentFactoryNotFound
}

func (fakeDefinitionsPeer) SetCurrentFactoryPointer(
	_ context.Context,
	request factorydefinitions.SetCurrentFactoryPointerRequest,
) (factorydefinitions.SetCurrentFactoryPointerResult, error) {
	if request.Name == "../evil" {
		return factorydefinitions.SetCurrentFactoryPointerResult{}, factorydefinitions.ErrInvalidNamedFactoryName
	}
	return factorydefinitions.SetCurrentFactoryPointerResult{Name: request.Name}, nil
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

func TestRootService_CatalogSlice_SuccessDetachedEntries(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{
		entries: []factorydefinitions.NamedFactoryListEntry{
			{
				Name:       "alpha",
				FactoryDir: "/factories/alpha",
				Current:    true,
			},
		},
	}

	listed, err := service.ListNamedFactories(
		context.Background(),
		factorydefinitions.ListNamedFactoriesRequest{RootDir: "/factories"},
	)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].Name != "alpha" {
		t.Fatalf("ListNamedFactories result = %#v, want alpha entry", listed)
	}

	got, err := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "alpha"},
	)
	if err != nil {
		t.Fatalf("GetNamedFactory: %v", err)
	}
	if got.Entry.Name != "alpha" || got.Entry.FactoryDir != "/factories/alpha" {
		t.Fatalf("GetNamedFactory result = %#v, want alpha identity facts", got)
	}

	pointer, err := service.GetCurrentFactoryPointer(
		context.Background(),
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: "/factories"},
	)
	if err != nil {
		t.Fatalf("GetCurrentFactoryPointer: %v", err)
	}
	if pointer.Name != "alpha" || pointer.FactoryDir != "/factories/alpha" {
		t.Fatalf("GetCurrentFactoryPointer result = %#v, want alpha current pointer", pointer)
	}
}

func TestRootService_CatalogSlice_TypedInvalidNameAndMissing(t *testing.T) {
	t.Parallel()

	var service factorydefinitions.Service = fakeDefinitionsPeer{}

	_, invalidErr := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "../evil"},
	)
	if !errors.Is(invalidErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatalf(
			"GetNamedFactory invalid-name error = %v, want %v",
			invalidErr,
			factorydefinitions.ErrInvalidNamedFactoryName,
		)
	}

	_, missingErr := service.GetNamedFactory(
		context.Background(),
		factorydefinitions.GetNamedFactoryRequest{RootDir: "/factories", Name: "missing"},
	)
	if !errors.Is(missingErr, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf(
			"GetNamedFactory missing error = %v, want %v",
			missingErr,
			factorydefinitions.ErrNamedFactoryNotFound,
		)
	}
	if errors.Is(missingErr, factorydefinitions.ErrInvalidNamedFactoryName) {
		t.Fatal("missing named Factory must not also match ErrInvalidNamedFactoryName")
	}
}
