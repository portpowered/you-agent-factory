package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type l1CaptureSamples struct {
	visible          l1Samples
	durable          l1Samples
	backlogHigh      int64
	observed         int
	oldestPendingAge time.Duration
	interval         l1CommitInterval
}

func measureL1Capture(t *testing.T, ctx context.Context, baseURL string, streams []*l1Stream) {
	t.Helper()
	started := time.Now()
	measurementCtx, stop := context.WithTimeout(ctx, l1Window)
	defer stop()
	results := make(chan l1CaptureSamples, len(streams))
	var monitors sync.WaitGroup
	for _, stream := range streams {
		monitors.Go(func() {
			results <- monitorL1Capture(t, ctx, measurementCtx, baseURL, stream.id, stream, started)
		})
	}
	var active, firstPage l1Samples
	// Explicit sample cadence bounds observer overhead. It is not readiness or
	// provider pacing; the independent 100 capture monitors run concurrently.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-measurementCtx.Done():
			monitors.Wait()
			close(results)
			active.report(t, "active-list", 250*time.Millisecond)
			firstPage.report(t, "first-logs-page", 500*time.Millisecond)
			reportL1Capture(t, results, streams, started, time.Since(started))
			return
		case <-ticker.C:
			queryStart := time.Now()
			body := fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions?history=active&maxResults=100", nil)
			active = append(active, time.Since(queryStart))
			var fleet factoryapi.ListWorkerSessionsResponse
			if err := json.Unmarshal(body, &fleet); err != nil || len(fleet.Sessions) != l1Active {
				t.Fatalf("active membership=%d err=%v", len(fleet.Sessions), err)
			}
			queryStart = time.Now()
			fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+streams[0].id+"/logs?limit=1", nil)
			firstPage = append(firstPage, time.Since(queryStart))
		}
	}
}

func monitorL1Capture(t *testing.T, ctx, measurementCtx context.Context, baseURL, id string, stream *l1Stream, started time.Time) l1CaptureSamples {
	t.Helper()
	result := l1CaptureSamples{interval: l1CommitInterval{start: started, end: started.Add(l1Window)}}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-measurementCtx.Done():
			return result
		case <-ticker.C:
			body := fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs?limit=500", nil)
			var page factoryapi.WorkerSessionLogPage
			if err := json.Unmarshal(body, &page); err != nil {
				t.Error(err)
				return result
			}
			if err := result.observe(stream, page, body); err != nil {
				t.Errorf("%s: %v", id, err)
				return result
			}
		}
	}
}

func (samples *l1CaptureSamples) observe(stream *l1Stream, page factoryapi.WorkerSessionLogPage, body []byte) error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	committed := int(page.CommittedPosition) - 2
	emitted := len(stream.emitted)
	if committed < samples.observed || committed > emitted {
		return fmt.Errorf("committed=%d observed=%d emitted=%d", committed, samples.observed, emitted)
	}
	observedAt := time.Now()
	// Measure each newly observed position, rather than only age of the latest
	// frame (which would conceal slow earlier records). Poll time is included;
	// each sample is a conservative public visibility/durability upper bound.
	for sequence := samples.observed + 1; sequence <= committed; sequence++ {
		if !bytes.Contains(body, []byte(fmt.Sprintf("l1 sequence=%06d", sequence))) {
			return fmt.Errorf("committed position %d absent from page", sequence)
		}
		lag := observedAt.Sub(stream.emitted[sequence-1])
		samples.interval.observe(sequence, stream.emitted[sequence-1], observedAt)
		samples.visible = append(samples.visible, lag)
		samples.durable = append(samples.durable, lag)
	}
	samples.observed = committed
	if committed < emitted {
		age := observedAt.Sub(stream.emitted[committed])
		if age > samples.oldestPendingAge {
			samples.oldestPendingAge = age
		}
	}
	// Fixed-workload bound only: validate every encoded envelope fits 4KiB,
	// reserve 64KiB for lifecycle/initial records. This is not a general queue
	// instrumentation claim or a substitute for the pressure/failure cell.
	backlog := int64(emitted-committed)*4096 + 65536
	if backlog > samples.backlogHigh {
		samples.backlogHigh = backlog
	}
	for _, event := range page.Events {
		encoded, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if len(encoded) > 4096 {
			return fmt.Errorf("event=%d bytes invalidates backlog estimator", len(encoded))
		}
	}
	return nil
}

func reportL1Capture(t *testing.T, results <-chan l1CaptureSamples, streams []*l1Stream, started time.Time, elapsed time.Duration) {
	t.Helper()
	var visible, durable l1Samples
	var backlog int64
	committed := 0
	var oldestPendingAge time.Duration
	for result := range results {
		committed += result.interval.count
		if len(result.visible) == 0 {
			t.Error("capture monitor returned no samples")
		}
		visible = append(visible, result.visible...)
		durable = append(durable, result.durable...)
		if result.backlogHigh > backlog {
			backlog = result.backlogHigh
		}
		if result.oldestPendingAge > oldestPendingAge {
			oldestPendingAge = result.oldestPendingAge
		}
	}
	visible.report(t, "visible-upper-bound", time.Second)
	durable.report(t, "durability-upper-bound", time.Second)
	if oldestPendingAge > time.Second {
		t.Errorf("maximum sampled oldest uncommitted age=%s exceeds 1s", oldestPendingAge)
	}
	if len(visible) > 0 && visible[len(visible)-1] > time.Second {
		t.Errorf("maximum visible upper bound=%s exceeds 1s", visible[len(visible)-1])
	}
	if len(durable) > 0 && durable[len(durable)-1] > time.Second {
		t.Errorf("maximum durable upper bound=%s exceeds 1s", durable[len(durable)-1])
	}
	count, emittedInWindow := 0, 0
	for _, stream := range streams {
		stream.mu.Lock()
		count += len(stream.emitted)
		for _, emitted := range stream.emitted {
			if !emitted.Before(started) && emitted.Before(started.Add(l1Window)) {
				emittedInWindow++
			}
		}
		stream.mu.Unlock()
	}
	rate := float64(committed) / l1Window.Seconds()
	t.Logf("emissionsIncludingDrain=%d intervalEmissions=%d intervalCommits=%d window=%s observerElapsed=%s achievedCommittedRate=%.2f/s backlogUpperBoundBytes=%d (fixed workload only)", count, emittedInWindow, committed, l1Window, elapsed, rate, backlog)
	if rate < 1000 {
		t.Errorf("achieved rate=%.2f/s want >=1000/s", rate)
	}
	if backlog > 8<<20 {
		t.Errorf("backlog bound=%d exceeds 8MiB", backlog)
	}
}

// Positions are session-local and monotonic. Only newly acknowledged workload
// records emitted and publicly committed inside the interval count; lifecycle,
// duplicate polling and post-window drain cannot inflate throughput.
type l1CommitInterval struct {
	start, end  time.Time
	last, count int
}

func (interval *l1CommitInterval) observe(sequence int, emitted, acknowledged time.Time) {
	if sequence <= interval.last {
		return
	}
	interval.last = sequence
	if !emitted.Before(interval.start) && emitted.Before(interval.end) && !acknowledged.Before(interval.start) && acknowledged.Before(interval.end) {
		interval.count++
	}
}
func TestL1MeasurementAccounting(t *testing.T) {
	start := time.Unix(100, 0)
	interval := l1CommitInterval{start: start, end: start.Add(10 * time.Second)}
	interval.observe(0, start, start) // opening/initial prefix
	interval.observe(1, start.Add(-time.Second), start.Add(time.Second))
	interval.observe(2, start.Add(time.Second), start.Add(2*time.Second))
	interval.observe(2, start.Add(time.Second), start.Add(3*time.Second))
	interval.observe(3, start.Add(9*time.Second), start.Add(10*time.Second))
	interval.observe(4, start.Add(11*time.Second), start.Add(12*time.Second))
	if interval.count != 1 {
		t.Fatalf("interval commits=%d want 1", interval.count)
	}
}
