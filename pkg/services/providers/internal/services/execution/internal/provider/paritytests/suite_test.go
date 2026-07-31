package providerparity

import (
	"context"
	"testing"
)

func TestCrossProviderParitySuite_Catalog(t *testing.T) {
	t.Parallel()

	for _, fixture := range Catalog() {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			t.Parallel()
			if err := AssertCrossProviderParityForFixture(context.Background(), fixture); err != nil {
				t.Fatal(err)
			}
		})
	}
}
