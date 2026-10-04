package consumer

import (
	renamed "m/pkg/services/b"
	"m/pkg/services/b/transports/http"
	"m/pkg/services/workers"
)

var alias = renamed.NewThing // want `service-construction: pkg/consumer -> pkg/services/b.NewThing`
func Closure() func() int    { return func() int { return renamed.EnsureThing() } } // want `service-construction: pkg/consumer -> pkg/services/b.EnsureThing`

var Pure = workers.NewCapabilities
var Lowercase = renamed.Newthing
var Adapter = othername.NewAdapter // want `service-construction: pkg/consumer -> pkg/services/b/transports/http.NewAdapter`
