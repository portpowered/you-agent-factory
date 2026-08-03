package service

import (
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// acpControlCanceledFailure reports the established canceled ExecuteFailure
// for an ACP attempt whose session/prompt turn honored a session/cancel
// notification and returned StopReasonCancelled instead of an RPC error,
// consistent with the ExecuteFailureKindCanceled every other cancellation
// path (native and ACP request-context cancellation) already normalizes to.
func acpControlCanceledFailure(id providers.ID) error {
	return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled, Message: fmt.Sprintf("ACP provider %q attempt was canceled", id)}
}
