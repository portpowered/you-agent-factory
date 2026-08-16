package internal

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// These helpers remain as a small composition seam for callers that assemble
// Runtime opening roles. The opening contracts are now neutral themselves, so
// no hosted-interface adapter or peer-visible compatibility cast is needed.
func adaptRuntimeLifecycle(lifecycle factoryruntime.RuntimeLifecycle) factoryruntime.RuntimeLifecycle {
	return lifecycle
}

func adaptRuntimeSidecars(sidecars factoryruntime.RuntimeSidecars) factoryruntime.RuntimeSidecars {
	return sidecars
}

func adaptRuntimeReplacementBuilder(builder factoryruntime.RuntimeReplacementBuilder) factoryruntime.RuntimeReplacementBuilder {
	return builder
}
