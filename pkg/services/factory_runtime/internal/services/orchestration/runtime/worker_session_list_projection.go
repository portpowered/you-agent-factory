package runtime

import workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"

// The scoped list already read these owner facts. Reuse them only within this
// request and physical Worker identity; the final merge retains the live owner's
// complete row. Historical identities select committed capture summaries.
// The next request must read newly committed facts.
func listedObservationIndex(rows []workersessions.Observation) map[string]workersessions.Observation {
	index := make(map[string]workersessions.Observation, len(rows))
	for _, row := range rows {
		index[row.WorkerSessionID] = row
	}
	return index
}
