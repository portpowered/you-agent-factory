package responsestream

import (
	"sync/atomic"
)

// PublicationDiagnostics summarizes operator-visible stream publication outcomes.
type PublicationDiagnostics struct {
	PublishedCount  int64
	CompactionCount int64
	LastCompaction  *CompactionSummary
}

// DiagnosticsObserver receives operator-visible publication outcomes such as
// compaction, truncation, or coalescing under retention pressure.
type DiagnosticsObserver func(summary CompactionSummary)

// Publisher appends internal response-stream events and reports degraded
// fidelity through diagnostics observers.
type Publisher struct {
	stream   *SessionResponseStream
	observer DiagnosticsObserver
	stats    PublicationDiagnostics
}

// NewPublisher constructs a publisher for one session-owned response stream.
func NewPublisher(stream *SessionResponseStream, observer DiagnosticsObserver) *Publisher {
	return &Publisher{
		stream:   stream,
		observer: observer,
	}
}

// Publish records one internal response-stream event and reports compaction
// when bounded retention drops older progress.
func (p *Publisher) Publish(event Event) Event {
	if p == nil || p.stream == nil {
		return event
	}
	stored, compaction := p.stream.Append(event)
	atomic.AddInt64(&p.stats.PublishedCount, 1)
	if compaction != nil {
		p.recordCompaction(*compaction)
	}
	return stored
}

// ReportCompaction records fidelity loss and emits a compaction signal event.
func (p *Publisher) ReportCompaction(summary CompactionSummary) {
	if p == nil || p.stream == nil {
		return
	}
	p.recordCompaction(summary)
}

func (p *Publisher) recordCompaction(summary CompactionSummary) {
	p.stream.appendCompactionSignal(Event{
		Kind:       EventKindCompactionSignal,
		Compaction: &summary,
	})
	atomic.AddInt64(&p.stats.CompactionCount, 1)
	p.stats.LastCompaction = &summary
	if p.observer != nil {
		p.observer(summary)
	}
}

func (s *SessionResponseStream) appendCompactionSignal(event Event) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendLocked(event, true)
}

// Diagnostics returns a snapshot of publication diagnostics for the stream.
func (p *Publisher) Diagnostics() PublicationDiagnostics {
	if p == nil {
		return PublicationDiagnostics{}
	}
	return PublicationDiagnostics{
		PublishedCount:  atomic.LoadInt64(&p.stats.PublishedCount),
		CompactionCount: atomic.LoadInt64(&p.stats.CompactionCount),
		LastCompaction:  p.stats.LastCompaction,
	}
}
