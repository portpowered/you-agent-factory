package listed

import "m/pkg/services/b"

var TestValue = b.NewThing // want `service-construction-test: pkg/listed -> pkg/services/b.NewThing`
