// Package invocation_policy owns the single private Definitions invocation
// resolver. Individual policy algorithms live below this package, but no
// policy fanout is published through the Definitions root.
package invocation_policy

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service is the one private invocation capability composed by Definitions.
type Service interface {
	ResolveInvocationDefinition(
		context.Context,
		factorydefinitions.ResolveInvocationDefinitionRequest,
	) (factorydefinitions.ResolveInvocationDefinitionResult, error)
}
