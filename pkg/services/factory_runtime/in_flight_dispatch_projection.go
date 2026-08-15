package factory

// ProjectCurrentlyInFlightDispatchCount reconciles the engine's raw dispatch
// counter with results that have been observed but are still retained until
// end-of-tick retirement. A matching result proves that one dispatch attempt
// is no longer currently in flight; unmatched results are ignored so an
// observation cannot hide bookkeeping that termination still needs to see.
// An empty active set is authoritative for the public view even if the raw
// counter has not reached its retirement value yet.
func ProjectCurrentlyInFlightDispatchCount(
	reportedCount int,
	activeDispatchIDs map[string]struct{},
	completedDispatchIDs []string,
) int {
	if reportedCount <= 0 || len(activeDispatchIDs) == 0 {
		return 0
	}

	completed := 0
	seen := make(map[string]struct{}, len(completedDispatchIDs))
	for _, dispatchID := range completedDispatchIDs {
		if _, alreadySeen := seen[dispatchID]; alreadySeen {
			continue
		}
		seen[dispatchID] = struct{}{}
		if _, active := activeDispatchIDs[dispatchID]; active {
			completed++
		}
	}
	if completed >= reportedCount {
		return 0
	}
	return reportedCount - completed
}
