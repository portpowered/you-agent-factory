package composition
import (
	_ "github.com/portpowered/infinite-you/pkg/initializer" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/platform/runtimeinput" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/factory_runtime" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/factory_definitions/scaffold" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/recordings/artifacts" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/recordings/events" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/recordings/projections" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/services/recordings/replay" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection" // want `not allowed from list`
	_ "github.com/portpowered/infinite-you/pkg/wire" // want `not allowed from list`
 )
