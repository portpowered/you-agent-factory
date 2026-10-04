// Package orchestrationowner projects the completed Runtime owner onto its
// public construction capabilities.
package orchestrationowner

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestration "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
)

// New exposes execution on the completed owner without another service graph.
func New(service orchestration.Service) factoryruntime.OrchestrationJavaScriptExecution {
	return service
}
