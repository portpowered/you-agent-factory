// Package invocation defines the owner-private Factory Session invocation
// capability consumed by the outer Factory Sessions runtime.
package invocation

import (
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

// Service owns request preparation, Work submission, result waiting, and
// invocation telemetry for one bound Factory Sessions runtime.
type Service interface {
	roles.SessionInvoker
	roles.CanonicalSessionInvoker
	roles.InvocationInputResolver
}
