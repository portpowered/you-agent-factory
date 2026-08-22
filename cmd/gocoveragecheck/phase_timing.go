package main

import (
	"fmt"
	"io"
	"time"
)

type coveragePhaseName string

const (
	coveragePhaseList         coveragePhaseName = "list"
	coveragePhasePlan         coveragePhaseName = "plan"
	coveragePhaseTest         coveragePhaseName = "test"
	coveragePhaseCanonicalize coveragePhaseName = "canonicalize"
	coveragePhaseEvaluate     coveragePhaseName = "evaluate"
	coveragePhaseManifest     coveragePhaseName = "manifest"
)

var coveragePhaseOrder = []coveragePhaseName{
	coveragePhaseList,
	coveragePhasePlan,
	coveragePhaseTest,
	coveragePhaseCanonicalize,
	coveragePhaseEvaluate,
	coveragePhaseManifest,
}

const (
	coveragePhaseStatusComplete = "complete"
	coveragePhaseStatusError    = "error"
	coveragePhaseStatusSkipped  = "skipped"
)

type coveragePhaseTiming struct {
	duration time.Duration
	started  time.Time
	active   bool
	recorded bool
	status   string
}

// coveragePhaseTimer emits one deterministic observation for each checker
// phase. The timer owns only measurement state; the caller still owns the
// phase boundaries and all coverage behavior.
type coveragePhaseTimer struct {
	now     func() time.Time
	sink    io.Writer
	phases  map[coveragePhaseName]*coveragePhaseTiming
	emitted bool
}

func newCoveragePhaseTimer(sink io.Writer) *coveragePhaseTimer {
	if sink == nil {
		sink = io.Discard
	}
	phases := make(map[coveragePhaseName]*coveragePhaseTiming, len(coveragePhaseOrder))
	for _, name := range coveragePhaseOrder {
		phases[name] = &coveragePhaseTiming{}
	}
	return &coveragePhaseTimer{
		now:    time.Now,
		sink:   sink,
		phases: phases,
	}
}

func (timer *coveragePhaseTimer) measure(name coveragePhaseName, action func() error) error {
	timer.begin(name)
	err := action()
	status := coveragePhaseStatusComplete
	if err != nil {
		status = coveragePhaseStatusError
	}
	timer.finish(name, status)
	return err
}

func (timer *coveragePhaseTimer) begin(name coveragePhaseName) {
	phase, ok := timer.phases[name]
	if !ok || phase.recorded || phase.active {
		return
	}
	phase.started = timer.now()
	phase.active = true
}

func (timer *coveragePhaseTimer) finish(name coveragePhaseName, status string) {
	phase, ok := timer.phases[name]
	if !ok || phase.recorded {
		return
	}
	if phase.active {
		phase.duration = timer.now().Sub(phase.started)
		phase.active = false
	}
	if phase.duration < 0 {
		phase.duration = 0
	}
	phase.status = status
	phase.recorded = true
}

func (timer *coveragePhaseTimer) finishRemaining(status string) {
	for _, name := range coveragePhaseOrder {
		phase := timer.phases[name]
		if phase.active {
			timer.finish(name, status)
			continue
		}
		if !phase.recorded {
			timer.finish(name, coveragePhaseStatusSkipped)
		}
	}
}

func (timer *coveragePhaseTimer) emit() {
	if timer == nil || timer.emitted {
		return
	}
	timer.finishRemaining(coveragePhaseStatusError)
	for _, name := range coveragePhaseOrder {
		phase := timer.phases[name]
		fmt.Fprintf(timer.sink, "gocoveragecheck phase timing: phase=%s duration=%.3fs status=%s\n", name, phase.duration.Seconds(), phase.status)
	}
	timer.emitted = true
}

func (cfg config) measureCoveragePhase(name coveragePhaseName, action func() error) error {
	if cfg.phaseTiming == nil {
		return action()
	}
	return cfg.phaseTiming.measure(name, action)
}

func (cfg config) beginCoveragePhase(name coveragePhaseName) {
	if cfg.phaseTiming != nil {
		cfg.phaseTiming.begin(name)
	}
}

func (cfg config) finishCoveragePhase(name coveragePhaseName, status string) {
	if cfg.phaseTiming != nil {
		cfg.phaseTiming.finish(name, status)
	}
}

func unitCoveragePhaseTimingEnabled(cfg config) bool {
	return cfg.suite == "" || cfg.suite == unitCoverageSuite
}
