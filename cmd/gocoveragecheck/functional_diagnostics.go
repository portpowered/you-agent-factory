package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
)

const functionalTimingSnapshotInterval = time.Second
const functionalCoverageSnapshotInterval = 5 * time.Second

// functionalTimingTracker records package lifecycle events independently from
// the buffered go test JSON. This lets a timeout snapshot describe packages
// that have started but never emitted a terminal event.
type functionalTimingTracker struct {
	mu sync.Mutex
	// expected preserves the sorted package order used by every snapshot.
	// expectedSet answers membership in constant time: the unit lane observes
	// hundreds of packages, so a linear scan per event grows with the square of
	// the lane size.
	expected    []string
	expectedSet map[string]struct{}
	started     map[string]time.Time
	terminals   map[string]functionalPackageTimingJSON
	invalid     bool
	now         func() time.Time
	runStarted  time.Time
}

func newFunctionalTimingTracker(expected []string, started time.Time) *functionalTimingTracker {
	unique := make(map[string]struct{}, len(expected))
	ordered := make([]string, 0, len(expected))
	for _, packageName := range expected {
		packageName = strings.TrimSpace(packageName)
		if packageName == "" {
			continue
		}
		if _, exists := unique[packageName]; exists {
			continue
		}
		unique[packageName] = struct{}{}
		ordered = append(ordered, packageName)
	}
	slices.Sort(ordered)
	return &functionalTimingTracker{
		expected:    ordered,
		expectedSet: unique,
		started:     make(map[string]time.Time, len(ordered)),
		terminals:   make(map[string]functionalPackageTimingJSON, len(ordered)),
		now:         time.Now,
		runStarted:  started,
	}
}

func (tracker *functionalTimingTracker) observe(event goTestTimingEvent) bool {
	packageName := strings.TrimSpace(event.Package)
	if packageName == "" || event.Test != "" {
		return false
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if !tracker.isExpectedLocked(packageName) {
		tracker.invalid = true
		return false
	}

	switch event.Action {
	case "start":
		if _, alreadyStarted := tracker.started[packageName]; alreadyStarted {
			return false
		}
		tracker.started[packageName] = tracker.now()
		return true
	case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
		if !validTimingElapsed(event.Elapsed) {
			tracker.invalid = true
			return false
		}
		if _, alreadyTerminal := tracker.terminals[packageName]; alreadyTerminal {
			tracker.invalid = true
			return false
		}
		entry := functionalPackageTimingJSON{
			Package: packageName,
			Seconds: event.Elapsed,
			Outcome: event.Action,
		}
		if event.Output != "" {
			entry.Reason = firstTimingFailureReason(event.Output)
		}
		tracker.terminals[packageName] = entry
		return true
	default:
		return false
	}
}

func (tracker *functionalTimingTracker) isExpectedLocked(packageName string) bool {
	_, expected := tracker.expectedSet[packageName]
	return expected
}

func (tracker *functionalTimingTracker) snapshot(complete bool, reason string, at time.Time) functionalTimingSummaryJSON {
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	packages := make([]functionalPackageTimingJSON, 0, len(tracker.terminals))
	states := make([]functionalPackageStateJSON, 0, len(tracker.expected))
	// A snapshot records package progress, not per-test rows. Tests is still
	// emitted as an empty array rather than left nil, because a nil slice
	// marshals to JSON null and a consumer reading a documented array field
	// cannot tell null apart from an unreadable document.
	tests := make([]functionalTestTimingJSON, 0)
	packageElapsed := 0.0
	for _, packageName := range tracker.expected {
		if terminal, ok := tracker.terminals[packageName]; ok {
			packages = append(packages, terminal)
			states = append(states, functionalPackageStateJSON{
				Package: packageName,
				Seconds: terminal.Seconds,
				State:   functionalPackageStateCompleted,
				Outcome: terminal.Outcome,
				Reason:  terminal.Reason,
			})
			packageElapsed += terminal.Seconds
			continue
		}

		seconds := 0.0
		state := functionalPackageStateUnobserved
		if packageStarted, ok := tracker.started[packageName]; ok {
			state = functionalPackageStateInFlight
			seconds = at.Sub(packageStarted).Seconds()
			if seconds < 0 {
				seconds = 0
			}
		}
		states = append(states, functionalPackageStateJSON{
			Package: packageName,
			Seconds: roundTimingSeconds(seconds),
			State:   state,
		})
		packageElapsed += seconds
	}

	allTerminal := len(tracker.terminals) == len(tracker.expected) && !tracker.invalid
	if wallSeconds := at.Sub(tracker.runStarted).Seconds(); wallSeconds >= 0 {
		return functionalTimingSummaryJSON{
			Version:                  functionalTimingSummaryVersion,
			Complete:                 complete && allTerminal,
			CaptureReason:            strings.TrimSpace(reason),
			WallSeconds:              roundTimingSeconds(wallSeconds),
			PackageElapsedSecondsSum: roundTimingSeconds(packageElapsed),
			ExpectedPackageCount:     len(tracker.expected),
			PackageCount:             len(packages),
			Packages:                 packages,
			PackageStates:            states,
			Tests:                    tests,
		}
	}
	return functionalTimingSummaryJSON{
		Version:              functionalTimingSummaryVersion,
		Complete:             false,
		CaptureReason:        strings.TrimSpace(reason),
		ExpectedPackageCount: len(tracker.expected),
		PackageCount:         len(packages),
		Packages:             packages,
		PackageStates:        states,
		Tests:                tests,
	}
}

func validTimingElapsed(seconds float64) bool {
	return seconds >= 0 && seconds < 1e12
}

// functionalTimingSnapshotter persists an incomplete document while the child
// is running and replaces it with the final buffered summary after completion.
// The ticker is deliberately short-lived and only writes the small structured
// timing document; it does not poll the child or extend the tier budget.
type functionalTimingSnapshotter struct {
	tracker               *functionalTimingTracker
	path                  string
	sink                  io.Writer
	sinkMu                *sync.Mutex
	onPublish             func() error
	writeMu               sync.Mutex
	writeErr              error
	lastCoveragePublishAt time.Time
	stop                  chan struct{}
	done                  chan struct{}
	stopOnce              sync.Once
}

func newFunctionalTimingSnapshotter(tracker *functionalTimingTracker, path string, sink io.Writer, sinkMu *sync.Mutex, callbacks ...func() error) *functionalTimingSnapshotter {
	var onPublish func() error
	if len(callbacks) > 0 {
		onPublish = callbacks[0]
	}
	snapshotter := &functionalTimingSnapshotter{
		tracker:   tracker,
		path:      strings.TrimSpace(path),
		sink:      sink,
		sinkMu:    sinkMu,
		onPublish: onPublish,
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
	go snapshotter.run()
	return snapshotter
}

func (snapshotter *functionalTimingSnapshotter) run() {
	defer close(snapshotter.done)
	ticker := time.NewTicker(functionalTimingSnapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			snapshotter.publish(false, "functional run still active", false)
		case <-snapshotter.stop:
			return
		}
	}
}

// observe records a package lifecycle event. The caller is the goroutine that
// drains the child go test pipe, so this path must stay cheap: a publish here
// serializes the whole run's snapshot and writes it to disk, and while that
// happens the pipe is not being read and every test binary blocks behind it.
//
// A streaming lane still publishes per event, because its progress lines are
// the live log a reader watches, and it observes a few dozen packages. A
// non-streaming lane has no sink to emit to, so persistence is left entirely to
// the one-second ticker in run(): the timeout snapshot it exists to protect is
// still on disk, without paying a full document write per event. The unit lane
// observes hundreds of packages, which is what makes the difference matter.
func (snapshotter *functionalTimingSnapshotter) observe(event goTestTimingEvent) {
	if !snapshotter.tracker.observe(event) {
		return
	}
	if snapshotter.sink == nil {
		return
	}
	snapshotter.publish(false, "functional run still active", true)
}

func (snapshotter *functionalTimingSnapshotter) finish(summary functionalTimingSummaryJSON, reason string) error {
	snapshotter.stopAndWait()
	snapshotter.writeMu.Lock()
	defer snapshotter.writeMu.Unlock()
	if !summary.Complete {
		partial := snapshotter.tracker.snapshot(false, reason, time.Now())
		partial.Tests = summary.Tests
		partial.TestCount = summary.TestCount
		partial.TestPassCount = summary.TestPassCount
		partial.TestFailCount = summary.TestFailCount
		partial.TestSkipCount = summary.TestSkipCount
		partial.Packages = summary.Packages
		partial.PackageCount = summary.PackageCount
		partial.PackageElapsedSecondsSum = summary.PackageElapsedSecondsSum
		partial.WallSeconds = summary.WallSeconds
		summary = partial
	}
	if snapshotter.path != "" {
		if err := writeFunctionalTimingSummaryJSON(snapshotter.path, summary); err != nil {
			snapshotter.rememberWriteErrorLocked(err)
		}
	}
	snapshotter.publishCoverageSnapshotLocked(true)
	if !summary.Complete || strings.TrimSpace(reason) != "" {
		snapshotter.emitSummary(summary, reason)
	}
	return snapshotter.writeErr
}

func (snapshotter *functionalTimingSnapshotter) stopAndWait() {
	snapshotter.stopOnce.Do(func() { close(snapshotter.stop) })
	<-snapshotter.done
}

func (snapshotter *functionalTimingSnapshotter) publish(complete bool, reason string, emit bool) {
	snapshotter.writeMu.Lock()
	defer snapshotter.writeMu.Unlock()
	summary := snapshotter.tracker.snapshot(complete, reason, time.Now())
	if snapshotter.path != "" {
		if err := writeFunctionalTimingSummaryJSON(snapshotter.path, summary); err != nil {
			snapshotter.rememberWriteErrorLocked(err)
		}
	}
	snapshotter.publishCoverageSnapshotLocked(false)
	if emit {
		snapshotter.emitSummary(summary, reason)
	}
}

func (snapshotter *functionalTimingSnapshotter) publishCoverageSnapshotLocked(force bool) {
	if snapshotter.onPublish == nil {
		return
	}
	now := time.Now()
	if !force && !snapshotter.lastCoveragePublishAt.IsZero() && now.Sub(snapshotter.lastCoveragePublishAt) < functionalCoverageSnapshotInterval {
		return
	}
	snapshotter.lastCoveragePublishAt = now
	if err := snapshotter.onPublish(); err != nil {
		snapshotter.rememberWriteErrorLocked(err)
	}
}

func (snapshotter *functionalTimingSnapshotter) emitSummary(summary functionalTimingSummaryJSON, reason string) {
	if snapshotter.sink == nil {
		return
	}
	write := func() {
		fmt.Fprintf(snapshotter.sink, "Functional timing snapshot: complete=%t packages=%d/%d wall=%.3fs reason=%s\n", summary.Complete, summary.PackageCount, summary.ExpectedPackageCount, summary.WallSeconds, reason)
		for _, state := range summary.PackageStates {
			if state.State == functionalPackageStateCompleted {
				continue
			}
			fmt.Fprintf(snapshotter.sink, "Functional package state: package=%s state=%s elapsed=%.3fs\n", state.Package, state.State, state.Seconds)
		}
	}
	if snapshotter.sinkMu == nil {
		write()
		return
	}
	snapshotter.sinkMu.Lock()
	defer snapshotter.sinkMu.Unlock()
	write()
}

func (snapshotter *functionalTimingSnapshotter) rememberWriteErrorLocked(err error) {
	if snapshotter.writeErr == nil {
		snapshotter.writeErr = err
	}
}

func (snapshotter *functionalTimingSnapshotter) writeError() error {
	snapshotter.writeMu.Lock()
	defer snapshotter.writeMu.Unlock()
	return snapshotter.writeErr
}
