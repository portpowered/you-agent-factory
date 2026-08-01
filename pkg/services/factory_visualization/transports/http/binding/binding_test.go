package binding_test

import (
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/http/binding"
)

func TestRequireRejectsNilAndTypedNilRoots(t *testing.T) {
	t.Parallel()

	for name, root := range map[string]binding.Root{
		"nil":       nil,
		"typed nil": (*typedNilRoot)(nil),
	} {
		name, root := name, root
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got, err := binding.New(root).Require(); got != nil || err == nil {
				t.Fatalf("Require() = (%v, %v), want (nil, error)", got, err)
			}
		})
	}
}

type typedNilRoot struct {
	factoryvisualization.Root
}

var _ factoryvisualization.Root = (*typedNilRoot)(nil)
