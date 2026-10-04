package external_test

import "m/pkg/services/b"

var Value = b.NewThing // want `service-construction-test: pkg/external_test -> pkg/services/b.NewThing`
