package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"strings"
)

type parsedFunctionalTimingEvents struct {
	packageOutcomes map[string]functionalPackageTimingJSON
	testOutcomes    map[string]functionalTestTimingJSON
	failureReasons  map[string]string
	complete        bool
}

func parseFunctionalTimingEvents(jsonOutput string, expectedPackages []string) parsedFunctionalTimingEvents {
	parsed := parsedFunctionalTimingEvents{
		packageOutcomes: make(map[string]functionalPackageTimingJSON, len(expectedPackages)),
		testOutcomes:    make(map[string]functionalTestTimingJSON),
		failureReasons:  make(map[string]string),
		complete:        true,
	}
	scanner := bufio.NewScanner(strings.NewReader(jsonOutput))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event goTestTimingEvent
		if err := json.Unmarshal(line, &event); err != nil {
			parsed.complete = false
			continue
		}
		if strings.TrimSpace(event.Package) == "" {
			continue
		}
		parsed.recordFailureReason(event)
		if !hasFunctionalTimingOutcome(event) {
			continue
		}
		if math.IsNaN(event.Elapsed) || math.IsInf(event.Elapsed, 0) || event.Elapsed < 0 {
			parsed.complete = false
			continue
		}
		if event.Test != "" {
			parsed.recordTest(event)
			continue
		}
		parsed.recordPackage(event)
	}
	if err := scanner.Err(); err != nil {
		parsed.complete = false
	}
	return parsed
}

func (parsed *parsedFunctionalTimingEvents) recordFailureReason(event goTestTimingEvent) {
	if event.Output == "" {
		return
	}
	key := timingEventKey(event.Package, event.Test)
	if reason := firstTimingFailureReason(event.Output); reason != "" && parsed.failureReasons[key] == "" {
		parsed.failureReasons[key] = reason
	}
}

func hasFunctionalTimingOutcome(event goTestTimingEvent) bool {
	switch event.Action {
	case timingOutcomePass, timingOutcomeFail, timingOutcomeSkip:
	default:
		return false
	}
	return true
}

func (parsed *parsedFunctionalTimingEvents) recordTest(event goTestTimingEvent) {
	// A slash identifies a Go subtest. The parent top-level test is emitted
	// separately and is the selector we need to inventory.
	if strings.Contains(event.Test, "/") {
		return
	}
	key := timingEventKey(event.Package, event.Test)
	if _, exists := parsed.testOutcomes[key]; exists {
		parsed.complete = false
		return
	}
	parsed.testOutcomes[key] = functionalTestTimingJSON{
		Package: event.Package,
		Test:    event.Test,
		Seconds: event.Elapsed,
		Outcome: event.Action,
		Reason:  parsed.failureReasons[key],
	}
}

func (parsed *parsedFunctionalTimingEvents) recordPackage(event goTestTimingEvent) {
	if _, exists := parsed.packageOutcomes[event.Package]; exists {
		parsed.complete = false
		return
	}
	key := timingEventKey(event.Package, "")
	parsed.packageOutcomes[event.Package] = functionalPackageTimingJSON{
		Package: event.Package,
		Seconds: event.Elapsed,
		Outcome: event.Action,
		Reason:  parsed.failureReasons[key],
	}
}
