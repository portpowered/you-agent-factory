package workersessions

import "github.com/portpowered/infinite-you/pkg/services/events"

// Topic returns the deterministic Events topic for Worker Session id:
// worker-session/<id>/events. Start commits the opening SESSION/STARTED
// record here before Workers invocation, and every terminal outcome commits
// its terminal SESSION record here after accepted output. Any caller reading
// or subscribing to a Worker Session's record stream derives the same topic
// identity through this one function rather than hand-formatting the string.
func Topic(id string) events.Topic {
	return events.Topic("worker-session/" + id + "/events")
}
