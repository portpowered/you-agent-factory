package parityfixtures_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/workers/provider/parityfixtures"
)

func TestCrossProviderParitySuite_Catalog(t *testing.T) {
	t.Parallel()

	for _, fixture := range parityfixtures.Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			if err := parityfixtures.AssertCrossProviderParityForFixture(context.Background(), fixture); err != nil {
				t.Fatal(err)
			}
		})
	}
}
