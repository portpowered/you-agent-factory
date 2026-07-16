package support

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
)

// ComposedFunctionalHTTPHost is the approved functional composition seam. It
// exposes only lifecycle controls and the supported HTTP boundary.
type ComposedFunctionalHTTPHost = testutil.FunctionalHTTPHost

type ComposedFunctionalHTTPHostConfig = testutil.FunctionalHTTPHostConfig

func StartComposedFunctionalHTTPHost(
	t *testing.T,
	cfg ComposedFunctionalHTTPHostConfig,
) *ComposedFunctionalHTTPHost {
	t.Helper()
	return testutil.StartFunctionalHTTPHost(t, cfg)
}
