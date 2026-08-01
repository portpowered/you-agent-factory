package wire

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ResolveCurrentFactoryDirectory returns the active Factory definition directory
// under rootDir using root Service catalog operations. Peers should depend on
// Service rather than CurrentFactoryDirectoryResolver.
func ResolveCurrentFactoryDirectory(
	ctx context.Context,
	service factorydefinitions.Service,
	rootDir string,
) (string, error) {
	pointer, err := service.GetCurrentFactoryPointer(
		ctx,
		factorydefinitions.GetCurrentFactoryPointerRequest{RootDir: rootDir},
	)
	if err != nil {
		return "", err
	}
	return pointer.FactoryDir, nil
}
