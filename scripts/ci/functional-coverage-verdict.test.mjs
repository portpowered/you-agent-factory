import assert from "node:assert/strict";
import test from "node:test";

import {
	classifyFunctionalCoverageRun,
	extractFunctionalCoverageVerdict,
	parseRecordedExitCode,
} from "./functional-coverage-verdict.mjs";

test("extracts the compact verdict and every package regression without raw stream noise", () => {
	const rawLine = `{"Time":"2026-08-20T00:00:00Z","Action":"run","Package":"github.com/portpowered/infinite-you/tests/functional"}`;
	const log = [
		rawLine,
		"Functional suite inventory: discovered-packages=3 observed-packages=3 (pass=1 fail=0 skip=0) top-level-tests=1 (pass=1 fail=0 skip=0) deferred-short-tests=0 wall=1.000s complete=true",
		"total: (statements) 70.0%",
		"Functional package coverage verdict:",
		"  floor violation: package=github.com/portpowered/infinite-you/pkg/alpha floor=75.0000% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements uncovered-blocks=3",
		"  package=github.com/portpowered/infinite-you/pkg/alpha coverage=70.0% floor=75.0% delta=-5.0pp gate=fail lane=functional",
		"  tally: measured-packages=2 gated-packages=2 below-floor=1 near-floor=0 gate-failures=1",
		"package coverage regression: package=github.com/portpowered/infinite-you/pkg/alpha lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements",
		"package coverage regression: package=github.com/portpowered/infinite-you/pkg/beta lane=functional expected-minimum=75.00% actual=60.0000% delta=-15.0000 percentage-points covered=6/10 statements",
		"make: *** [functional-test-viz] Error 1",
	].join("\n");

	const extracted = extractFunctionalCoverageVerdict(log);
	assert.equal(extracted.foundInventory, true);
	assert.equal(extracted.hasCoverageGateFailure, true);
	assert.equal(
		extracted.lines.filter((line) =>
			line.startsWith("package coverage regression:"),
		).length,
		2,
	);
	assert.equal(extracted.text.includes(rawLine), false);
	assert.equal(extracted.text.includes("make: ***"), false);
	assert.match(extracted.text, /Functional suite inventory:/);
});

test("retains the advisory banner and distinguishes report-only findings from failed tests", () => {
	const extracted = extractFunctionalCoverageVerdict(
		[
			"!!! COVERAGE FLOOR POLICY: advisory !!!",
			"Package floors and missing-manifest findings are report-only during the test-corpus rebuild.",
			"Set -package-floor-policy=blocking to restore blocking enforcement.",
			"Functional suite inventory: discovered-packages=2 observed-packages=2 complete=true",
			"package coverage regression: package=p lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points",
			"coverage manifest missing entry: package=service-root lane=functional",
			"coverage not evaluated: package=unmeasured lane=functional (no measurement in profile)",
			"Go coverage 80.0% meets minimum 33.1%.",
		].join("\n"),
	);
	assert.equal(extracted.hasAdvisoryPolicy, true);
	assert.equal(extracted.hasAdvisoryFindings, true);
	assert.equal(extracted.hasOrdinaryTestFailure, false);
	assert.match(extracted.text, /COVERAGE FLOOR POLICY: advisory/);
	assert.match(
		extracted.text,
		/coverage manifest missing entry: package=service-root/,
	);
});

test("captures the exact recorded gocoveragecheck exit code", () => {
	assert.equal(parseRecordedExitCode("17\n", "test exit file"), 17);
	assert.throws(
		() => parseRecordedExitCode("17 18", "test exit file"),
		/one non-negative integer/,
	);
	assert.throws(
		() => parseRecordedExitCode("256", "test exit file"),
		/outside the supported/,
	);
});

test("retains the uppercase final green coverage gate in the compact extract", () => {
	const extracted = extractFunctionalCoverageVerdict(
		[
			"Functional suite inventory: discovered-packages=1 observed-packages=1 complete=true",
			"Functional package coverage verdict:",
			"  tally: measured-packages=1 gated-packages=1 below-floor=0 near-floor=0 gate-failures=0",
			"Go coverage 80.0% meets minimum 33.1%.",
		].join("\n"),
	);
	assert.equal(extracted.hasGreenGate, true);
	assert.match(extracted.text, /Go coverage 80\.0% meets minimum 33\.1%\./);
});

test("defers ordinary test and coverage-gate failures but propagates timeout and infrastructure outcomes", () => {
	const testFailure = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 1,
		log: "Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=0 fail=1 skip=0)\ncoverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed",
	});
	assert.equal(testFailure.outcome, "test-failure");
	assert.equal(testFailure.shouldDeferFailure, true);

	const missingMeasurement = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 1,
		log: "Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=1 fail=0 skip=0)\ncoverage not evaluated: package=p lane=functional (no measurement in profile)",
	});
	assert.equal(missingMeasurement.outcome, "incomplete");
	assert.equal(missingMeasurement.extraction.hasOrdinaryTestFailure, false);

	const gateFailure = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 1,
		log: "Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=1 fail=0 skip=0)\nFunctional package coverage verdict:\n  tally: gate-failures=1\npackage coverage regression: package=p",
	});
	assert.equal(gateFailure.outcome, "coverage-gate-failure");
	assert.equal(gateFailure.shouldDeferFailure, true);

	const timeout = classifyFunctionalCoverageRun({
		commandExitCode: 124,
		gocoverageExitCode: 1,
		log: "",
	});
	assert.equal(timeout.outcome, "timeout");
	assert.equal(timeout.shouldDeferFailure, false);
	assert.equal(timeout.exitCode, 124);

	const infrastructure = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 1,
		log: "missing tool: go was not found",
	});
	assert.equal(infrastructure.outcome, "infrastructure-failure");
	assert.equal(infrastructure.shouldDeferFailure, false);
});

test("classifies advisory-only coverage as successful and keeps a real test failure authoritative", () => {
	const advisory = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 0,
		log: [
			"!!! COVERAGE FLOOR POLICY: advisory !!!",
			"Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=1 fail=0 skip=0)",
			"package coverage regression: package=p lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points",
			"Go coverage 70.0% meets minimum 33.1%.",
		].join("\n"),
	});
	assert.equal(advisory.outcome, "advisory");
	assert.equal(advisory.shouldDeferFailure, false);
	assert.equal(advisory.exitCode, 0);

	const failedTest = classifyFunctionalCoverageRun({
		commandExitCode: 0,
		gocoverageExitCode: 1,
		log: [
			"!!! COVERAGE FLOOR POLICY: advisory !!!",
			"Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=0 fail=1 skip=0)",
			"coverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed",
		].join("\n"),
	});
	assert.equal(failedTest.outcome, "test-failure");
	assert.equal(failedTest.shouldDeferFailure, true);
});
