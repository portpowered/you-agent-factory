package scaffold_test

import (
	"io"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold"
)

// Init keeps owner-local scaffold tests focused on scaffold behavior while
// constructing the owned implementation with explicit local effects.
func Init(config factorydefinitions.ScaffoldConfig) error {
	initializer, err := scaffold.New(platformfilesystem.Local{}, io.Discard)
	if err != nil {
		return err
	}
	return initializer.Init(config)
}
