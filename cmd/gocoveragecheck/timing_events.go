package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

type parsedFunctionalTimingEvents struct {
	packageOutcomes  map[string]functionalPackageTimingJSON
	testOutcomes     map[string]functionalTestTimingJSON
	observedPackages map[string]struct{}
	failureReasons   map[string]string
	complete         bool
}

func parseFunctionalTimingEvents(jsonOutput string, expectedPackages []string) parsedFunctionalTimingEvents {
	parsed := parsedFunctionalTimingEvents{
		packageOutcomes:  make(map[string]functionalPackageTimingJSON, len(expectedPackages)),
		testOutcomes:     make(map[string]functionalTestTimingJSON),
		observedPackages: make(map[string]struct{}, len(expectedPackages)),
		failureReasons:   make(map[string]string),
		complete:         true,
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
		parsed.observedPackages[event.Package] = struct{}{}
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

func validateFunctionalRuntimeInventory(jsonOutput string, expected functionalTestInventory) error {
	parsed := parseFunctionalTimingEvents(jsonOutput, expected.Packages)
	expectedPackages := make(map[string]struct{}, len(expected.Packages))
	for _, packagePath := range expected.Packages {
		expectedPackages[packagePath] = struct{}{}
	}
	expectedTests := functionalInventoryTestKeys(expected)
	observedTests := make(map[string]struct{}, len(parsed.testOutcomes))
	for key := range parsed.testOutcomes {
		observedTests[key] = struct{}{}
	}

	missingPackages, unexpectedPackages := functionalInventorySetDiff(expectedPackages, parsed.observedPackages)
	missingTests, unexpectedTests := functionalInventorySetDiff(expectedTests, observedTests)
	if parsed.complete && len(missingPackages) == 0 && len(unexpectedPackages) == 0 && len(missingTests) == 0 && len(unexpectedTests) == 0 {
		return nil
	}

	details := make([]string, 0, 5)
	if !parsed.complete {
		details = append(details, "test2json event stream was incomplete or malformed")
	}
	if len(missingPackages) > 0 {
		details = append(details, "missing-packages="+formatFunctionalInventoryKeys(missingPackages))
	}
	if len(unexpectedPackages) > 0 {
		details = append(details, "unexpected-packages="+formatFunctionalInventoryKeys(unexpectedPackages))
	}
	if len(missingTests) > 0 {
		details = append(details, "missing-tests="+formatFunctionalInventoryKeys(missingTests))
	}
	if len(unexpectedTests) > 0 {
		details = append(details, "unexpected-tests="+formatFunctionalInventoryKeys(unexpectedTests))
	}
	return fmt.Errorf("functional runtime inventory mismatch: %s", strings.Join(details, "; "))
}

func functionalInventoryTestKeys(inventory functionalTestInventory) map[string]struct{} {
	keys := make(map[string]struct{})
	for packagePath, tests := range inventory.Tests {
		for _, testName := range tests {
			keys[timingEventKey(packagePath, testName)] = struct{}{}
		}
	}
	return keys
}

func functionalInventorySetDiff(expected, observed map[string]struct{}) ([]string, []string) {
	missing := make([]string, 0)
	for key := range expected {
		if _, ok := observed[key]; !ok {
			missing = append(missing, key)
		}
	}
	unexpected := make([]string, 0)
	for key := range observed {
		if _, ok := expected[key]; !ok {
			unexpected = append(unexpected, key)
		}
	}
	slices.Sort(missing)
	slices.Sort(unexpected)
	return missing, unexpected
}

func formatFunctionalInventoryKeys(keys []string) string {
	formatted := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) == 2 {
			formatted = append(formatted, parts[0]+"#"+parts[1])
			continue
		}
		formatted = append(formatted, key)
	}
	return fmt.Sprintf("%d[%s]", len(formatted), strings.Join(formatted, ","))
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
