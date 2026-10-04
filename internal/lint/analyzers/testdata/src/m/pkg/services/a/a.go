package a

import (
	_ "m/pkg/services/b"
	_ "m/pkg/services/b/sub" // want `service-subpackage: pkg/services/a -> pkg/services/b/sub`
	_ "m/pkg/wire"           // want `application-graph: pkg/services/a -> pkg/wire`
)
