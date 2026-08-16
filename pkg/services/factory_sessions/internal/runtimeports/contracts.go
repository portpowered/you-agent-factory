package runtimeports

import factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"

// These names are the private Factory Sessions view of the Runtime opening
// edge. The aliases keep the retained implementation adapter in one place;
// callers should retain the opaque RuntimeBinding and not publish these
// mechanics in a service contract.
type RuntimeInstance = factoryruntime.HostedInstance
type RuntimeHandle = factoryruntime.HostedHandle
type RuntimeLifecycle = factoryruntime.Lifecycle
type RuntimeSidecarService = factoryruntime.Sidecars
type RuntimeReplacementBuilder = factoryruntime.ReplacementBuilder
