package runtimebinding

import "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"

// The runtime-binding package is the only Factory Sessions compatibility edge
// that still knows the pre-binding host vocabulary. Keep these aliases here so
// session state and domain operations retain only the opaque RuntimeBinding;
// the hosted implementation can be removed from this edge once the private
// instance_host lifecycle operations are exposed through the Runtime root.
type RuntimeInstance = runtimeports.RuntimeInstance
type RuntimeHandle = runtimeports.RuntimeHandle
type RuntimeLifecycle = runtimeports.RuntimeLifecycle
type RuntimeSidecarService = runtimeports.RuntimeSidecarService
type RuntimeReplacementBuilder = runtimeports.RuntimeReplacementBuilder
