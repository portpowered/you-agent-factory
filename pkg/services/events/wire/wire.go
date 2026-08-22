// Package wire is the Events service composition boundary.
//
// Wire performs construction only, returns the singular events.Service root
// interface, and starts no background lifecycle work: the returned
// implementation is inert until canonical application construction folds its
// Close(context.Context) error shutdown role into the process shutdown path.
// Parent-private in-memory store construction stays inside the owner service
// assembly path; peers depend on Service rather than store internals.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	events "github.com/portpowered/infinite-you/pkg/services/events"
	internalservice "github.com/portpowered/infinite-you/pkg/services/events/internal/service"
)

// NewService constructs an inert, process-scoped Events root using the
// default bounded-retention policy. logger is optional and defaults to a
// no-op logger when omitted, matching the repository's optional-logger
// construction convention. Construction starts no background lifecycle work
// and performs no durable write.
func NewService(logger ...logging.Logger) (events.Service, error) {
	return internalservice.New(logger...), nil
}
