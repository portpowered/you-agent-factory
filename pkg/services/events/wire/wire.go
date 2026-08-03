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

// NewServiceWithRetention constructs an inert, process-scoped Events root
// bounded to at most maxRetainedRecordsPerTopic records per topic; a
// non-positive value falls back to the same default bounded-retention policy
// NewService uses. This exists so canonical construction can accept an
// explicit per-process retention override (see pkg/wire's
// provideEventsService) without widening events.Service or exposing the
// internal Store constructor outside this package's construction boundary.
func NewServiceWithRetention(maxRetainedRecordsPerTopic int, logger ...logging.Logger) (events.Service, error) {
	return internalservice.NewWithRetention(maxRetainedRecordsPerTopic, logger...), nil
}
