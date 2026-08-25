import assert from "node:assert/strict";
import { chmodSync, existsSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	classifyFunctionalCoverageRun,
	extractFunctionalCoverageVerdict,
	parseRecordedExitCode,
} from "./functional-coverage-verdict.mjs";

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
const runnerPath = "scripts/ci/run-functional-test-viz.sh";
const publishPath = "scripts/ci/publish-functional-coverage-verdict.sh";

function requireBash(t) {
	if (process.platform === "win32") {
		t.skip("POSIX workflow smoke test runs on the Linux CI runner");
		return false;
	}
	const result = spawnSync("bash", ["-c", "exit 0"], { encoding: "utf8" });
	if (result.error) {
		t.skip(`Bash workflow smoke test requires bash: ${result.error.message}`);
		return false;
	}
	return true;
}

function writeFakeMake(t, outcome) {
	const path = join(mkdtempSync(join(tmpdir(), "functional-verdict-")), "fake-make.sh");
	const body = `#!/bin/sh
exit_file=""
for argument in "$@"; do
  case "$argument" in
    FUNCTIONAL_GOCOVERAGE_EXIT_FILE=*) exit_file="\${argument#*=}" ;;
  esac
done
case "${outcome}" in
  green)
    printf '%s\\n' 'Functional suite inventory: discovered-packages=2 observed-packages=2 (pass=2 fail=0 skip=0) top-level-tests=2 (pass=2 fail=0 skip=0) deferred-short-tests=0 wall=1.000s complete=true'
    printf '%s\\n' 'total: (statements) 80.0%'
    printf '%s\\n' 'Functional package coverage verdict:'
    printf '%s\\n' '  floor violations: none'
    printf '%s\\n' '  package=github.com/portpowered/infinite-you/pkg/alpha coverage=80.0% floor=75.0% delta=+5.0pp gate=pass lane=functional'
    printf '%s\\n' '  tally: measured-packages=1 gated-packages=1 below-floor=0 near-floor=0 gate-failures=0'
    printf '%s\\n' 'Go coverage 80.0% meets minimum 33.1%.'
    printf '%s\\n' 'full functional stream remains in command.log'
    printf '%s\\n' '0' > "$exit_file"
    ;;
  advisory)
    printf '%s\\n' '!!! COVERAGE FLOOR POLICY: advisory !!!'
    printf '%s\\n' 'Package floors and missing-manifest findings are report-only during the test-corpus rebuild.'
    printf '%s\\n' 'Set -package-floor-policy=blocking to restore blocking enforcement.'
    printf '%s\\n' 'Functional suite inventory: discovered-packages=2 observed-packages=2 (pass=2 fail=0 skip=0) top-level-tests=2 (pass=2 fail=0 skip=0) deferred-short-tests=0 wall=1.000s complete=true'
    printf '%s\\n' 'total: (statements) 70.0%'
    printf '%s\\n' 'Functional package coverage verdict:'
    printf '%s\\n' '  floor violation: package=github.com/portpowered/infinite-you/pkg/alpha floor=75.0000% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements uncovered-blocks=3'
    printf '%s\\n' '  package=github.com/portpowered/infinite-you/pkg/alpha coverage=70.0% floor=75.0% delta=-5.0pp gate=fail lane=functional'
    printf '%s\\n' '  tally: measured-packages=1 gated-packages=1 below-floor=1 near-floor=0 gate-failures=0'
    printf '%s\\n' 'package coverage regression: package=github.com/portpowered/infinite-you/pkg/alpha lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements'
    printf '%s\\n' 'coverage manifest missing entry: package=github.com/portpowered/infinite-you/pkg/services/factory_runtime lane=functional (measured service has no root manifest entry; record one entry for the service root)'
    printf '%s\\n' 'coverage not evaluated: package=github.com/portpowered/infinite-you/pkg/beta lane=functional (no measurement in profile)'
    printf '%s\\n' 'Go coverage 70.0% meets minimum 33.1%.'
    printf '%s\\n' '0' > "$exit_file"
    ;;
  gate)
    printf '%s\\n' 'full functional stream remains in command.log'
    printf '%s\\n' 'Functional suite inventory: discovered-packages=2 observed-packages=2 (pass=2 fail=0 skip=0) top-level-tests=2 (pass=2 fail=0 skip=0) deferred-short-tests=0 wall=1.000s complete=true'
    printf '%s\\n' 'total: (statements) 70.0%'
    printf '%s\\n' 'Functional package coverage verdict:'
    printf '%s\\n' '  floor violation: package=github.com/portpowered/infinite-you/pkg/alpha floor=75.0000% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements uncovered-blocks=3'
    printf '%s\\n' '  package=github.com/portpowered/infinite-you/pkg/alpha coverage=70.0% floor=75.0% delta=-5.0pp gate=fail lane=functional'
    printf '%s\\n' '  tally: measured-packages=1 gated-packages=1 below-floor=1 near-floor=0 gate-failures=1'
    printf '%s\\n' 'package coverage regression: package=github.com/portpowered/infinite-you/pkg/alpha lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements'
    printf '%s\\n' 'package coverage regression: package=github.com/portpowered/infinite-you/pkg/beta lane=functional expected-minimum=75.00% actual=60.0000% delta=-15.0000 percentage-points covered=6/10 statements'
    printf '%s\\n' '1' > "$exit_file"
    ;;
  test)
    printf '%s\\n' 'full functional stream remains in command.log'
    printf '%s\\n' 'Functional suite inventory: discovered-packages=2 observed-packages=2 (pass=1 fail=1 skip=0) top-level-tests=2 (pass=1 fail=1 skip=0) deferred-short-tests=0 wall=1.000s complete=true'
    printf '%s\\n' 'coverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed'
    printf '%s\\n' '1' > "$exit_file"
    ;;
  infra)
    printf '%s\\n' 'missing tool: go was not found'
    printf '%s\\n' '1' > "$exit_file"
    ;;
esac
`;
	writeFileSync(path, body, "utf8");
	chmodSync(path, 0o755);
	return path;
}

function runFunctionalRunner(t, outcome) {
	const artifactRoot = mkdtempSync(join(tmpdir(), `functional-verdict-${outcome}-`));
	const makePath = writeFakeMake(t, outcome);
	return {
		artifactRoot,
		result: spawnSync("bash", [runnerPath], {
			cwd: repositoryRoot,
			env: {
				...process.env,
				FUNCTIONAL_TEST_VIZ_DIR: artifactRoot,
				FUNCTIONAL_TEST_BUDGET: "5s",
				FUNCTIONAL_TEST_TIER: "test",
				FUNCTIONAL_TEST_TRIGGER: "node-test",
				MAKE_BIN: makePath.replaceAll("\\", "/"),
				NODE_BIN: process.execPath,
			},
			encoding: "utf8",
		}),
	};
}

test("extracts only the compact verdict contract without verbose diagnostics", () => {
	const rawLine = `{"Time":"2026-08-20T00:00:00Z","Action":"run","Package":"github.com/portpowered/infinite-you/tests/functional"}`;
	const log = [
		rawLine,
		"tests/ functional-package timing:",
		"total: (statements) 70.0%",
		"pkg/ functional coverage:",
		"  package=github.com/portpowered/infinite-you/tests/functional/alpha elapsed=1.000s outcome=pass",
		"  floor violation: package=github.com/portpowered/infinite-you/pkg/alpha floor=75.0000% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements uncovered-blocks=3",
		"    pkg/alpha/file.go:41 (2 statements)",
		"  package=github.com/portpowered/infinite-you/pkg/alpha coverage=70.0% floor=75.0% delta=-5.0pp gate=fail lane=functional",
		"  package=github.com/portpowered/infinite-you/pkg/alpha coverage=70.0% floor=75.0% delta=-5.0pp gate=fail lane=functional",
		"  tally: measured-packages=2 gated-packages=2 below-floor=1 near-floor=0 gate-failures=1",
		"package coverage regression: package=github.com/portpowered/infinite-you/pkg/alpha lane=functional expected-minimum=75.00% actual=70.0000% delta=-5.0000 percentage-points covered=7/10 statements",
		"package coverage regression: package=github.com/portpowered/infinite-you/pkg/beta lane=functional expected-minimum=75.00% actual=60.0000% delta=-15.0000 percentage-points covered=6/10 statements",
		"coverage manifest missing entry: package=service-root lane=functional",
		"coverage not evaluated: package=unmeasured lane=functional (no measurement in profile)",
		"make: *** [functional-test-viz] Error 1",
	].join("\n");

	const extracted = extractFunctionalCoverageVerdict(log);
	assert.equal(extracted.foundInventory, true);
	assert.equal(extracted.hasCoverageGateFailure, true);
	assert.equal(extracted.lines.filter((line) => line.startsWith("  package=")).length, 1);
	assert.equal(new Set(extracted.lines).size, extracted.lines.length);
	assert.equal(extracted.text.includes(rawLine), false);
	assert.equal(extracted.text.includes("make: ***"), false);
	assert.doesNotMatch(extracted.text, /floor violation|package coverage regression|coverage manifest missing|coverage not evaluated|uncovered blocks|file\.go:/);
	assert.match(extracted.text, /pkg\/ functional coverage:/);
	assert.match(extracted.text, /tests\/ functional-package timing:/);
	assert.match(extracted.text, /  tally:/);
	assert.equal(extracted.text.includes("Functional suite inventory:"), false);
});

test("uses the concise test-failure diagnostic when no timing report was completed", () => {
	const extracted = extractFunctionalCoverageVerdict(
		[
			"raw successful child output is omitted",
			"coverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed",
		].join("\n"),
	);
	assert.equal(extracted.foundInventory, true);
	assert.equal(extracted.hasOrdinaryTestFailure, true);
	assert.match(extracted.text, /coverage not evaluated: 1 failed tests observed/);
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
	assert.match(extracted.text, /Functional suite inventory:/);
	assert.doesNotMatch(extracted.text, /package coverage regression|coverage manifest missing entry|coverage not evaluated/);
});

test("captures the exact recorded gocoveragecheck exit code", () => {
	assert.equal(parseRecordedExitCode("17\n", "test exit file"), 17);
	assert.throws(() => parseRecordedExitCode("17 18", "test exit file"), /one non-negative integer/);
	assert.throws(() => parseRecordedExitCode("256", "test exit file"), /outside the supported/);
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

	const timeout = classifyFunctionalCoverageRun({ commandExitCode: 124, gocoverageExitCode: 1, log: "" });
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

test("green runner remains successful and records its compact verdict", (t) => {
	if (!requireBash(t)) return;
	const { artifactRoot, result } = runFunctionalRunner(t, "green");
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const verdictPath = join(artifactRoot, "functional-coverage-verdict.txt");
	assert.ok(existsSync(verdictPath));
	const verdict = readFileSync(verdictPath, "utf8");
	assert.match(verdict, /Go coverage 80\.0% meets minimum/);
	assert.equal(verdict.match(/  package=.*lane=functional/g)?.length, 1);
	assert.doesNotMatch(verdict, /floor violation|uncovered blocks/);
	assert.equal(readFileSync(join(artifactRoot, "gocoveragecheck-exit-code.txt"), "utf8").trim(), "0");
	assert.doesNotMatch(result.stdout, /Functional package coverage verdict:|  package=.*lane=functional|Go coverage .* meets minimum/);
});

test("advisory runner remains successful and retains policy plus findings", (t) => {
	if (!requireBash(t)) return;
	const { artifactRoot, result } = runFunctionalRunner(t, "advisory");
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const verdict = readFileSync(join(artifactRoot, "functional-coverage-verdict.txt"), "utf8");
	assert.match(verdict, /Functional coverage outcome: advisory/);
	assert.match(verdict, /COVERAGE FLOOR POLICY: advisory/);
	assert.ok(verdict.includes("package=github.com/portpowered/infinite-you/pkg/alpha"));
	assert.doesNotMatch(verdict, /floor violation|coverage manifest missing entry|coverage not evaluated|uncovered blocks/);
	assert.doesNotMatch(result.stdout, /Functional package coverage verdict:|  package=.*lane=functional|Go coverage .* meets minimum/);
	assert.equal(readFileSync(join(artifactRoot, "gocoveragecheck-exit-code.txt"), "utf8").trim(), "0");
});

test("recorded nonzero verdict fails only in the final verdict step", (t) => {
	if (!requireBash(t)) return;
	const { artifactRoot, result } = runFunctionalRunner(t, "gate");
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);

	const summaryPath = join(artifactRoot, "summary.md");
	const verdict = readFileSync(join(artifactRoot, "functional-coverage-verdict.txt"), "utf8");
	const publish = spawnSync("bash", [publishPath], {
		cwd: repositoryRoot,
		env: {
			...process.env,
			FUNCTIONAL_TEST_VIZ_DIR: artifactRoot,
			GITHUB_STEP_SUMMARY: summaryPath,
		},
		encoding: "utf8",
	});
	assert.equal(publish.status, 1, `${publish.stdout}\n${publish.stderr}`);
	assert.equal(publish.stdout.includes(verdict), true);
	assert.equal(readFileSync(summaryPath, "utf8").includes(verdict), true);
	assert.equal(`${result.stdout}\n${publish.stdout}`.match(/  package=.*lane=functional/g)?.length, 1);
	assert.doesNotMatch(`${result.stdout}\n${publish.stdout}`, /floor violation|uncovered blocks/);
});

test("infrastructure failure keeps the full runner step red", (t) => {
	if (!requireBash(t)) return;
	const { artifactRoot, result } = runFunctionalRunner(t, "infra");
	assert.equal(result.status, 1, `${result.stdout}\n${result.stderr}`);
	assert.equal(existsSync(join(artifactRoot, "functional-coverage-verdict.txt")), false);
});
