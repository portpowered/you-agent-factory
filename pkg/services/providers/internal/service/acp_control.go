package service

import (
	"context"
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
)

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
// session/prompt turn looked truthfully live a moment ago. It is a
// deliberately racy pre-filter used only to decide whether the registry
// should remove this identity's registration for a Cancel request at all
// (so a Cancel that arrives before or after the live window leaves the
// registration untouched for a later attempt); signal is the atomic source
// of truth for whether the resulting outcome was genuinely accepted.
func (control *acpAttemptControl) supports(action providers.ControlAction) bool {
	return action == providers.ControlActionCancel && control.acp.Cancelable(control.canonical, control.attemptID)
}

// signal delegates to acp.Service.TryCancel, which atomically re-derives
// liveness at the instant it acts and grounds its accepted result in the
// attempt's real recorded outcome rather than in supports's earlier
// observation - so a natural completion racing this call reports
// accepted=false instead of a false ControlOutcomeCompleted. TryCancel
// already distinguishes its two non-nil-error cases by construction: a
// genuine delivery failure is wrapped in providers.ErrControlSignalFailed,
// while ctx ending before the outcome could be observed surfaces as the
// caller's own unwrapped ctx.Err(); signal only adds identifying context,
// preserving errors.Is for both underneath.
func (control *acpAttemptControl) signal(ctx context.Context) (bool, error) {
	accepted, err := control.acp.TryCancel(ctx, control.canonical, control.attemptID)
	if err != nil {
		return false, fmt.Errorf("deliver cancel to provider %q attempt %q: %w", control.canonical, control.attemptID, err)
	}
	return accepted, nil
}
