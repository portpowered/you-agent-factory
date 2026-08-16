package runtimeports

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// These names are the private Factory Sessions view of the Runtime opening
// edge. They point at neutral Factory Runtime contracts; callers should retain
// the opaque RuntimeBinding and not publish these mechanics in a service
// contract.
type RuntimeInstance = factoryruntime.RuntimeRecord
type RuntimeHandle = factoryruntime.RuntimeRun
type RuntimeLifecycle = factoryruntime.RuntimeLifecycle
type RuntimeSidecarService = factoryruntime.RuntimeSidecars
type RuntimeReplacementBuilder = factoryruntime.RuntimeReplacementBuilder
