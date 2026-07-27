package factorysessions

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// RuntimeStop releases one Factory Session runtime sidecar group.
type RuntimeStop = func(context.Context) error

// BoundProcessComponents are protocol and presentation components adapted at
// the composition boundary. Their product lifecycle order is deliberately not
// represented here; LifecyclePlanOperation owns that policy.
type BoundProcessComponents struct {
	Transport     lifecycle.Component
	Visualization lifecycle.Component
}

// DirectJavaScriptApplication is the Factory Sessions-owned lifecycle plan
// for one raw workflow-file run.
type DirectJavaScriptApplication struct {
	Plan lifecycle.Plan
}

// RuntimeHostRequest contains the process-hosting values needed after a
// Factory Session runtime has started. It deliberately excludes the broader
// process configuration and edge aggregates.
type RuntimeHostRequest struct {
	Directory   string
	RuntimeMode interfaces.RuntimeMode
	WorkFile    string
	MockWorkers bool
	Host        string
	Port        int
	AutoPort    bool
}

// RuntimeHostBinding is the endpoint selected by the external HTTP host.
type RuntimeHostBinding struct {
	Host string
	Port int
}

// RuntimeHostObserver receives the selected endpoint for presentation at the
// process boundary. It does not own listener or server lifecycle.
type RuntimeHostObserver func(RuntimeHostBinding)
