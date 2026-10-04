package listed

import "m/pkg/services/b"

var Value = b.NewThing
var Other = b.EnsureThing // want `service-construction: pkg/listed -> pkg/services/b.EnsureThing`
