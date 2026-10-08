package runtime

import (
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// The scoped list already read these owner facts. Reuse them only within this
// request and for the same Worker identity and association; the final merge
// retains the live owner's complete row. Historical identities still use the
// existing enrichment path. The next request must read newly committed facts.
func listedObservationIndex(rows []workersessions.Observation) map[string]workersessions.Observation {
	index := make(map[string]workersessions.Observation, len(rows))
	for _, row := range rows {
		index[row.WorkerSessionID] = row
	}
	return index
}

func listedObservationMatches(row workersessions.Observation, ref providers.SessionRef, live map[string]workersessions.Observation) bool {
	listed, ok := live[row.WorkerSessionID]
	return ok && listed.ProviderSessionAvailable && listed.ProviderSession == ref
}
