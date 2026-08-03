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
// (see acp.Service.Claim); before or after that span the attempt stays live
// and untouched, so a later Cancel can still succeed once the span opens.
// Pause and Terminate are never supported: the only ACP protocol seam
// available cancels one session/prompt turn, it does not pause a turn or
// shut down the daemon.
//
// generation is nil until supports(Cancel) successfully claims one; signal
// then delivers only to that captured generation, never re-deriving
// liveness from canonical/attemptID strings. This is what keeps a claimed
// control bound to the exact execution generation it was claimed from: even
// if that generation completes and a later execution reuses the identical
// canonical/attemptID identity before signal runs, signal still targets only
// the originally captured generation and cannot be redirected to the
// replacement. supports and signal are always called sequentially by the
// same caller (registry.claim, then ControlAttempt) with no intervening
// concurrent access, so generation needs no synchronization of its own.
type acpAttemptControl struct {
	acp        acp.Service
	canonical  providers.ID
	attemptID  string
	generation acp.Generation
}

var _ liveAttemptControl = (*acpAttemptControl)(nil)

// supports atomically claims the bound attempt's exact live generation, if
// its session/prompt turn is truthfully live right now, capturing it into
// generation for signal to use. It is used only to decide whether the
// registry should remove this identity's registration for a Cancel request
// at all (so a Cancel that arrives before or after the live window leaves
// the registration untouched for a later attempt); signal remains the atomic
// source of truth for whether the resulting outcome was genuinely accepted,
// grounded in the exact generation captured here rather than in a later
// re-derivation by identity.
func (control *acpAttemptControl) supports(action providers.ControlAction) bool {
	if action != providers.ControlActionCancel {
		return false
	}
	generation, ok := control.acp.Claim(control.canonical, control.attemptID)
	if !ok {
		return false
	}
	control.generation = generation
	return true
}

// signal delivers to the exact generation supports captured, via
// acp.Service.TryCancel, which grounds its accepted result in that
// generation's real recorded outcome - so a natural completion racing this
// call, or a generation that already ended before this call runs, reports
// accepted=false instead of a false ControlOutcomeCompleted, and can never
// observe a different (for example later, identity-reusing) generation's
// outcome. TryCancel already distinguishes its two non-nil-error cases by
// construction: a genuine delivery failure is wrapped in
// providers.ErrControlSignalFailed, while ctx ending before the outcome
// could be observed surfaces as the caller's own unwrapped ctx.Err(); signal
// only adds identifying context, preserving errors.Is for both underneath.
func (control *acpAttemptControl) signal(ctx context.Context) (bool, error) {
	accepted, err := control.acp.TryCancel(ctx, control.generation)
	if err != nil {
		return false, fmt.Errorf("deliver cancel to provider %q attempt %q: %w", control.canonical, control.attemptID, err)
	}
	return accepted, nil
}
