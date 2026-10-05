package workersessions

import (
	"encoding/base64"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/events"
)

// Topic returns the deterministic Events topic for a Worker Session. Factory
// callers supply the owning Factory Session to isolate retained replay records.
// Direct callers omit the scope and retain worker-session/<id>/events.
// The supplied Worker identity remains unchanged in records and public cursors.
func Topic(id string, factorySessionIDs ...string) events.Topic {
	factorySessionID := ""
	if len(factorySessionIDs) > 0 {
		factorySessionID = strings.TrimSpace(factorySessionIDs[0])
	}
	if factorySessionID == "" {
		return events.Topic("worker-session/" + id + "/events")
	}
	return events.Topic("factory-worker-session/" +
		base64.RawURLEncoding.EncodeToString([]byte(factorySessionID)) + "/" +
		base64.RawURLEncoding.EncodeToString([]byte(id)) + "/events")
}
