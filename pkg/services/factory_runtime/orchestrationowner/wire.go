// Package orchestrationowner is the composition-root entrypoint for the
// parent-private Factory Runtime orchestration owner.
package orchestrationowner

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	orchestrationwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/wire"
)

// New constructs the Runtime JavaScript orchestration execute/resume port.
func New(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflows,
) factoryruntime.OrchestrationJavaScriptExecution {
	if workflows == nil {
		return nil
	}
	return orchestrationwire.New(newID, workflows, workflows)
}
