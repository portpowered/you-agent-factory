package service_test

import (
	"context"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// testSessionPublisher supplies an explicit session to the retained progress
// operation with its immutable observer and Worker identity.
type testSessionPublisher struct {
	progress workersessions.ProviderSessionObservationPublisher
	observer workersessions.Service
	workerID string
	next     workers.ProgressPublisher
}

func newTestSessionPublisher(observer workersessions.Service, workerID string, next workers.ProgressPublisher) *testSessionPublisher {
	return &testSessionPublisher{observer: observer, workerID: workerID, next: next}
}
func (p *testSessionPublisher) Publish(fragment workers.ProgressFragment) {
	if fragment.Correlation.DispatchID == "" {
		fragment.Correlation.DispatchID = fragment.DispatchID
	}
	if fragment.Correlation.AttemptID == "" {
		fragment.Correlation.AttemptID = fragment.DispatchID
	}
	if err := p.progress.PublishWorkerSessionProgress(context.Background(), p.observer, p.workerID, fragment); err != nil {
		return
	}
	if p.next != nil {
		p.next(fragment)
	}
}
