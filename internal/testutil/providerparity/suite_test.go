package providerparity_test

import (
	"context"
	"testing"

	parityfixtures "github.com/portpowered/infinite-you/internal/testutil/providerparity"
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
