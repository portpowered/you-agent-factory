package invocationreturnpolicy

// explicit_last_resort.go holds the explicit invocation return policy's
// unlinked last-resort match and the cross-invocation scoping that keeps it
// from reaching into another invocation's work. It sits beside
// primary_result.go rather than inside it so that file stays within the
// repository's own size limit.

import "strings"

// collectUniqueExplicitTerminalMatches is the explicit policy's last resort,
// for terminal work this invocation's Factory produced with no submission or
// trace link back to the submitted item -- the case neither the scope tier
// nor the trace tier can see.
//
// It is deliberately not scoped to this invocation's own work, because having
// no link to that work is the whole reason it runs. It is scoped *away* from
// every other invocation's work instead: a session serves many invocations in
// turn, and while this one is still running, the previous one's answer is
// already sitting in TerminalWorkByID. Matching it would end this
// invocation's wait early and return the previous turn's answer as this
// turn's -- which is exactly what a multi-turn ACP conversation does on its
// second and every later turn.
func collectUniqueExplicitTerminalMatches(
	state InvocationWorldState,
	requestID string,
	cfg *InvocationReturnConfig,
) []WorkItem {
	foreignWorkIDs, foreignTraceIDs := otherInvocationWork(state, requestID)
	matches := make([]WorkItem, 0, 1)
	for _, terminalWorkID := range sortedTerminalWorkIDs(state.TerminalWorkByID) {
		terminal := state.TerminalWorkByID[terminalWorkID]
		if isFailedTerminalWork(terminal) {
			continue
		}
		if _, claimed := foreignWorkIDs[terminal.WorkItem.ID]; claimed {
			continue
		}
		if _, claimed := foreignTraceIDs[strings.TrimSpace(terminal.WorkItem.TraceID)]; claimed {
			continue
		}
		if !explicitPrimaryResultMatches(terminal.WorkItem, cfg) {
			continue
		}
		matches = append(matches, terminal.WorkItem)
	}
	return matches
}

// otherInvocationWork returns the work and trace identities claimed by every
// invocation on this session except requestID. Work carrying no request or
// trace identity at all belongs to neither set, so it stays eligible for the
// unlinked last resort above.
func otherInvocationWork(
	state InvocationWorldState,
	requestID string,
) (workIDs map[string]struct{}, traceIDs map[string]struct{}) {
	workIDs = make(map[string]struct{})
	traceIDs = make(map[string]struct{})
	for otherRequestID, request := range state.WorkRequestsByID {
		if otherRequestID == requestID {
			continue
		}
		if traceID := strings.TrimSpace(request.TraceID); traceID != "" {
			traceIDs[traceID] = struct{}{}
		}
		for _, item := range request.WorkItems {
			if item.ID != "" {
				workIDs[item.ID] = struct{}{}
			}
			if traceID := strings.TrimSpace(item.TraceID); traceID != "" {
				traceIDs[traceID] = struct{}{}
			}
		}
	}
	// A trace this invocation also carries is not foreign: a session that
	// reuses one trace across its invocations must not exclude its own work.
	if request, ok := state.WorkRequestsByID[requestID]; ok {
		delete(traceIDs, strings.TrimSpace(request.TraceID))
		for _, item := range request.WorkItems {
			delete(traceIDs, strings.TrimSpace(item.TraceID))
			delete(workIDs, item.ID)
		}
	}
	delete(traceIDs, "")
	return workIDs, traceIDs
}
