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
