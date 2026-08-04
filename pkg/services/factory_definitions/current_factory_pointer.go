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

// ResolveNamedFactoryOperation resolves one canonical Factory name across
// project and global roots, applying existing project-before-global
// precedence.
type ResolveNamedFactoryOperation func(context.Context, ResolveNamedFactoryRequest) (ResolveNamedFactoryResult, error)

// ResolveCurrentFactoryLocationOperation resolves the current Factory
// location for one root, following the current pointer when present and
// otherwise falling back to a directly authored factory.json.
type ResolveCurrentFactoryLocationOperation func(context.Context, ResolveCurrentFactoryLocationRequest) (ResolveCurrentFactoryLocationResult, error)

// CatalogPathsService is Factory Definitions' narrow, stateless, read-only
// capability for available Factory target metadata and named/current Factory
// location resolution. It intentionally excludes authoring, compilation,
// validation, snapshot, distribution, runtime, and session operations; peers
// that need only catalog and path reads should depend on this type instead
// of the full Service root. Its three fields are the exact narrow operation
// types Factory Definitions already publishes at its root
// (EffectiveFactoryCatalogOperation, ResolveNamedFactoryOperation,
// ResolveCurrentFactoryLocationOperation) bundled into one struct value, the
// same shape EffectiveFactoryCatalogDiscovery already uses at this root for
// a narrow read-only capability -- not a named interface, so publishing it
// here adds no service-root-interface-count debt.
type CatalogPathsService struct {
	ListEffectiveFactories        EffectiveFactoryCatalogOperation
	ResolveNamedFactory           ResolveNamedFactoryOperation
	ResolveCurrentFactoryLocation ResolveCurrentFactoryLocationOperation
}
