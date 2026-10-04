package i

import (
	_ "m/pkg/services/b"
	_ "m/pkg/services/b/sub" // want `initializer-service: pkg/initializer/i -> pkg/services/b/sub`
	_ "m/pkg/transports/t"   // want `initializer-transport: pkg/initializer/i -> pkg/transports/t`
)
