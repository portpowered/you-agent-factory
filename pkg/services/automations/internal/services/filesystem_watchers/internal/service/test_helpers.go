package service

import (
	"context"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

type recordingSubmitter struct {
	mu           sync.Mutex
	workRequests []work.WorkRequest
	submitted    chan struct{}
	err          error
}

func (m *recordingSubmitter) Submit(_ context.Context, request work.WorkRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workRequests = append(m.workRequests, cloneWorkRequest(request))
	if m.submitted != nil {
		select {
		case m.submitted <- struct{}{}:
		default:
		}
	}
	return m.err
}

func (m *recordingSubmitter) getWorkRequests() []work.WorkRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]work.WorkRequest, len(m.workRequests))
	for i := range m.workRequests {
		out[i] = cloneWorkRequest(m.workRequests[i])
	}
	return out
}

func (m *recordingSubmitter) submitCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.workRequests)
}

func cloneWorkRequest(request work.WorkRequest) work.WorkRequest {
	out := request
	out.Works = make([]work.Work, len(request.Works))
	for i := range request.Works {
		out.Works[i] = request.Works[i]
		if payload, ok := request.Works[i].Payload.([]byte); ok {
			out.Works[i].Payload = append([]byte(nil), payload...)
		}
		if request.Works[i].Tags != nil {
			out.Works[i].Tags = make(map[string]string, len(request.Works[i].Tags))
			for key, value := range request.Works[i].Tags {
				out.Works[i].Tags[key] = value
			}
		}
	}
	out.Relations = append([]work.WorkRelation(nil), request.Relations...)
	return out
}
