package service

import (
	"context"
	"fmt"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
)

// acpCancelTimeout bounds only the outbound session/cancel notification
// send; it does not bound waiting for the attempt to observe cancellation
// (acp.Service.Cancel blocks on that separately, unbounded, matching the
// native control's <-done wait).
const acpCancelTimeout = 500 * time.Millisecond

// acpAttemptControl is the control handle bound for one live ACP provider
// attempt. Unlike nativeAttemptControl, whether Cancel is truthfully
// supported depends on live ACP session state that exists only for the span
// between the bound attempt's session/prompt call starting and returning
// (see acp.Service.Cancelable); before or after that span the attempt stays
// live and untouched, so a later Cancel can still succeed once the span
// opens. Pause and Terminate are never supported: the only ACP protocol
// seam available cancels one session/prompt turn, it does not pause a turn
// or shut down the daemon.
type acpAttemptControl struct {
	acp       acp.Service
	canonical providers.ID
	attemptID string
}

var _ liveAttemptControl = (*acpAttemptControl)(nil)

// supports reports, without any side effect, whether the bound attempt's
// session/prompt turn is truthfully live right now.
func (control *acpAttemptControl) supports(action providers.ControlAction) bool {
	return action == providers.ControlActionCancel && control.acp.Cancelable(control.canonical, control.attemptID)
}

// signal delivers the session/cancel protocol notification to the bound
// attempt's exact session and blocks until its session/prompt call has
// returned. Callers must only invoke signal after supports(Cancel) reported
// true under the same registry claim, which guarantees the identity was
// truthfully live at that atomic instant; a natural completion racing this
// call is a harmless no-op handled by acp.Service.Cancel. A non-nil return
// reports a genuine delivery failure (for example the outbound notification
// timing out against acpCancelTimeout, or the ACP connection itself
// erroring) - distinct from "unsupported", since supports(Cancel) already
// proved the capability was truthfully live at claim time.
func (control *acpAttemptControl) signal() error {
	ctx, cancel := context.WithTimeout(context.Background(), acpCancelTimeout)
	defer cancel()
	if err := control.acp.Cancel(ctx, control.canonical, control.attemptID); err != nil {
		return fmt.Errorf(
			"%w: deliver cancel to provider %q attempt %q: %w",
			providers.ErrControlSignalFailed, control.canonical, control.attemptID, err,
		)
	}
	return nil
}
