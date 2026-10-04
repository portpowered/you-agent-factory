//go:build boundaryfixtureexcluded

package ignoredimports

import (
	_ "m/pkg/services/b"
	_ "m/pkg/services/b/sub" // want `service-subpackage: pkg/services/ignoredimports -> pkg/services/b/sub`
	_ "m/pkg/services/edges" // want `constructed-service-edges: pkg/services/ignoredimports -> pkg/services/edges`
)
