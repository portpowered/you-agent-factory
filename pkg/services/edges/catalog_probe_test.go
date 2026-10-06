package edges

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestT7MergePreservesAndReplacesReadinessProjection(t *testing.T) {
	t.Parallel()
	missing := errors.New("selected readiness source unavailable")
	defaults := Edges{ProviderCatalogProbe: func(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
		descriptor.Readiness = providers.ReadinessReady
		return descriptor, nil
	}}
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserved", true: "replaced"}[replace], func(t *testing.T) {
			t.Parallel()
			replacement := Edges{}
			if replace {
				replacement.ProviderCatalogProbe = func(_ context.Context, descriptor providers.Descriptor) (providers.Descriptor, error) {
					return descriptor, missing
				}
			}
			merged := Merge(defaults, replacement)
			descriptor, err := merged.ProviderCatalogProbe(context.Background(), providers.Descriptor{ID: providers.IDCodex})
			if replace {
				if !errors.Is(err, missing) {
					t.Fatalf("replacement readiness = %v", err)
				}
			} else if err != nil || descriptor.Readiness != providers.ReadinessReady {
				t.Fatalf("preserved readiness = %#v, %v", descriptor, err)
			}
		})
	}
}
