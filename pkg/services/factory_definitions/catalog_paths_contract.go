package factorydefinitions

import "context"

// CatalogPathsService is Factory Definitions' narrow, stateless, read-only
// capability for available Factory target metadata and named/current Factory
// location resolution. It intentionally excludes authoring, compilation,
// validation, snapshot, distribution, runtime, and session operations; peers
// that need only catalog and path reads depend on this interface instead of
// the full Service root.
type CatalogPathsService interface {
	ListEffectiveFactories(context.Context, ListEffectiveFactoriesRequest) (ListEffectiveFactoriesResult, error)
	ResolveNamedFactory(context.Context, ResolveNamedFactoryRequest) (ResolveNamedFactoryResult, error)
	ResolveCurrentFactoryLocation(context.Context, ResolveCurrentFactoryLocationRequest) (ResolveCurrentFactoryLocationResult, error)
}

// ResolveNamedFactoryOperation is the function-typed form of
// CatalogPathsService's ResolveNamedFactory. It lets Wire composition inject
// the existing catalog collaborator directly into narrow read-only
// construction without exposing the collaborator's own implementation type.
type ResolveNamedFactoryOperation func(context.Context, ResolveNamedFactoryRequest) (ResolveNamedFactoryResult, error)

// ResolveCurrentFactoryLocationRequest selects the root whose current Factory
// location to resolve.
type ResolveCurrentFactoryLocationRequest struct {
	RootDir string
}

// ResolveCurrentFactoryLocationResult carries the resolved current Factory
// directory: the current-pointer target when a pointer exists at RootDir,
// otherwise a root directly authored with a regular factory.json.
type ResolveCurrentFactoryLocationResult struct {
	FactoryDir string
}
