package support

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/internal/functionalhost"
)

// ComposedFunctionalHTTPHost is the approved functional composition seam. It
// exposes only lifecycle controls and the supported HTTP boundary.
type ComposedFunctionalHTTPHost = functionalhost.FunctionalHTTPHost

type ComposedFunctionalHTTPHostConfig = functionalhost.FunctionalHTTPHostConfig

func StartComposedFunctionalHTTPHost(
	t *testing.T,
	cfg ComposedFunctionalHTTPHostConfig,
) *ComposedFunctionalHTTPHost {
	t.Helper()
	return functionalhost.StartFunctionalHTTPHost(t, cfg)
}
