package internal_test

import (
	"context"
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

func validCatalogPathsCollaborators() (
	factorydefinitions.EffectiveFactoryCatalogOperation,
	factorydefinitions.ResolveNamedFactoryOperation,
	factorydefinitions.CurrentFactoryDirectoryResolver,
) {
	listEffective := func(
		context.Context,
		factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		return factorydefinitions.ListEffectiveFactoriesResult{}, nil
	}
	resolveNamedFactory := func(
		context.Context,
		factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		return factorydefinitions.ResolveNamedFactoryResult{}, nil
	}
	resolveCurrentDir := func(string) (string, error) {
		return "", nil
	}
	return listEffective, resolveNamedFactory, resolveCurrentDir
}

func TestNewCatalogPathsServiceRejectsMissingCollaborators(t *testing.T) {
	t.Parallel()

	listEffective, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	if _, err := factoryinternal.NewCatalogPathsService(nil, resolveNamedFactory, resolveCurrentDir); err == nil {
		t.Fatal("NewCatalogPathsService with nil listEffective = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, nil, resolveCurrentDir); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveNamedFactory = nil error, want error")
	}
	if _, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, nil); err == nil {
		t.Fatal("NewCatalogPathsService with nil resolveCurrentDir = nil error, want error")
	}
}

func TestNewCatalogPathsServicePerformsNoIOAtConstruction(t *testing.T) {
	t.Parallel()

	panicky := func(context.Context, factorydefinitions.ListEffectiveFactoriesRequest) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		panic("listEffective invoked during inert construction")
	}
	panickyResolve := func(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error) {
		panic("resolveNamedFactory invoked during inert construction")
	}
	panickyCurrent := func(string) (string, error) {
		panic("resolveCurrentDir invoked during inert construction")
	}

	if _, err := factoryinternal.NewCatalogPathsService(panicky, panickyResolve, panickyCurrent); err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}
}

func TestCatalogPathsServiceListEffectiveFactoriesForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ListEffectiveFactoriesResult{
		Entries: []factorydefinitions.EffectiveFactoryCatalogEntry{{Name: "alpha"}},
	}
	var gotRequest factorydefinitions.ListEffectiveFactoriesRequest
	listEffective := func(
		_ context.Context,
		request factorydefinitions.ListEffectiveFactoriesRequest,
	) (factorydefinitions.ListEffectiveFactoriesResult, error) {
		gotRequest = request
		return want, nil
	}
	_, resolveNamedFactory, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ListEffectiveFactoriesRequest{ProjectRoot: "/project", GlobalRoot: "/global"}
	got, err := service.ListEffectiveFactories(context.Background(), request)
	if err != nil {
		t.Fatalf("ListEffectiveFactories: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ListEffectiveFactories forwarded request = %+v, want %+v", gotRequest, request)
	}
	if len(got.Entries) != 1 || got.Entries[0].Name != "alpha" {
		t.Fatalf("ListEffectiveFactories result = %+v, want the collaborator's result", got)
	}
}

func TestCatalogPathsServiceResolveNamedFactoryForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	want := factorydefinitions.ResolveNamedFactoryResult{
		Resolution: factorydefinitions.NamedFactoryResolution{Name: "alpha", FactoryDir: "/project/alpha"},
	}
	var gotRequest factorydefinitions.ResolveNamedFactoryRequest
	resolveNamedFactory := func(
		_ context.Context,
		request factorydefinitions.ResolveNamedFactoryRequest,
	) (factorydefinitions.ResolveNamedFactoryResult, error) {
		gotRequest = request
		return want, nil
	}
	listEffective, _, resolveCurrentDir := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	request := factorydefinitions.ResolveNamedFactoryRequest{ProjectRoot: "/project", GlobalRoot: "/global", Name: "alpha"}
	got, err := service.ResolveNamedFactory(context.Background(), request)
	if err != nil {
		t.Fatalf("ResolveNamedFactory: unexpected error: %v", err)
	}
	if gotRequest != request {
		t.Fatalf("ResolveNamedFactory forwarded request = %+v, want %+v", gotRequest, request)
	}
	if got != want {
		t.Fatalf("ResolveNamedFactory result = %+v, want %+v", got, want)
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationForwardsToCollaborator(t *testing.T) {
	t.Parallel()

	var gotRootDir string
	resolveCurrentDir := func(rootDir string) (string, error) {
		gotRootDir = rootDir
		return "/project/current", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	got, err := service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{
		RootDir: "/project",
	})
	if err != nil {
		t.Fatalf("ResolveCurrentFactoryLocation: unexpected error: %v", err)
	}
	if gotRootDir != "/project" {
		t.Fatalf("ResolveCurrentFactoryLocation forwarded RootDir = %q, want %q", gotRootDir, "/project")
	}
	if got.FactoryDir != "/project/current" {
		t.Fatalf("ResolveCurrentFactoryLocation FactoryDir = %q, want %q", got.FactoryDir, "/project/current")
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationHonorsCancelledContext(t *testing.T) {
	t.Parallel()

	called := false
	resolveCurrentDir := func(string) (string, error) {
		called = true
		return "", nil
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.ResolveCurrentFactoryLocation(ctx, factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is context.Canceled", err)
	}
	if called {
		t.Fatal("ResolveCurrentFactoryLocation invoked the collaborator despite a cancelled context")
	}
}

func TestCatalogPathsServiceResolveCurrentFactoryLocationPropagatesTypedError(t *testing.T) {
	t.Parallel()

	resolveCurrentDir := func(string) (string, error) {
		return "", factorydefinitions.ErrFactoryLayoutNotFound
	}
	listEffective, resolveNamedFactory, _ := validCatalogPathsCollaborators()

	service, err := factoryinternal.NewCatalogPathsService(listEffective, resolveNamedFactory, resolveCurrentDir)
	if err != nil {
		t.Fatalf("NewCatalogPathsService: unexpected error: %v", err)
	}

	_, err = service.ResolveCurrentFactoryLocation(context.Background(), factorydefinitions.ResolveCurrentFactoryLocationRequest{RootDir: "/project"})
	if !errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		t.Fatalf("ResolveCurrentFactoryLocation error = %v, want errors.Is ErrFactoryLayoutNotFound", err)
	}
}
