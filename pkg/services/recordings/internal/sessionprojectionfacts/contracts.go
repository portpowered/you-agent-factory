// Package sessionprojectionfacts contains the small, dependency-neutral
// contract shared by the recordings ledger and live Factory Session reads.
package sessionprojectionfacts

import interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// SessionProjectionFacts contains the event-derived facts needed by live
// Factory Session reads.
type SessionProjectionFacts struct {
	PendingHumanApprovals map[string]interfaces.FactoryWorldHumanApproval
	JavaScriptRuntime     *interfaces.FactorySessionJavaScriptRuntimeState
	SessionBracket        *interfaces.FactoryWorldSessionBracketState
}
