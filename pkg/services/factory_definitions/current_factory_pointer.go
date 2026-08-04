package factorydefinitions

import "context"

// ResolveCurrentFactoryDirectory returns the active Factory definition directory
// under rootDir using root Service catalog operations. Peers should depend on
// Service rather than CurrentFactoryDirectoryResolver.
func ResolveCurrentFactoryDirectory(
	ctx context.Context,
	service Service,
	rootDir string,
) (string, error) {
	pointer, err := service.GetCurrentFactoryPointer(
		ctx,
		GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		return "", err
	}
	return pointer.FactoryDir, nil
}

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

// CatalogPathsService is Factory Definitions' narrow, stateless, read-only
// capability for available Factory target metadata and named/current Factory
// location resolution. It intentionally excludes authoring, compilation,
// validation, snapshot, distribution, runtime, and session operations; peers
// that need only catalog and path reads should depend on this interface
// instead of the full Service root. Production Wire returns its unexported,
// stateless implementation through this owner-root interface.
type CatalogPathsService interface {
	ListEffectiveFactories(context.Context, ListEffectiveFactoriesRequest) (ListEffectiveFactoriesResult, error)
	ResolveNamedFactory(context.Context, ResolveNamedFactoryRequest) (ResolveNamedFactoryResult, error)
	ResolveCurrentFactoryLocation(context.Context, ResolveCurrentFactoryLocationRequest) (ResolveCurrentFactoryLocationResult, error)
}
