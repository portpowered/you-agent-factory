package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	internal "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/internal"
)

func New(descriptors []catalog.Descriptor, locator platformprocess.ExecutableLocator) (catalog.Service, error) {
	return internal.New(descriptors, locator)
}
