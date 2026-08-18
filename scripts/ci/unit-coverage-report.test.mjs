import assert from "node:assert/strict";
import test from "node:test";

import { BACKEND_LINT_COMMENT_MARKER } from "./backend-lint-report.mjs";
import { FUNCTIONAL_COVERAGE_COMMENT_MARKER } from "./functional-coverage-comment.mjs";
import {
	UNIT_COVERAGE_COMMENT_MARKER,
	UNIT_COVERAGE_PACKAGE_LIMIT,
	UNIT_COVERAGE_SLOWEST_TEST_LIMIT,
	UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT,
	UNIT_COVERAGE_SUMMARY_SLOWEST_TEST_LIMIT,
	UNIT_COVERAGE_VIOLATION_LIMIT,
	renderUnitCoverageAvailability,
	renderUnitCoverageComment,
	renderUnitCoverageJobSummary,
	summarizeUnitCoverage,
	upsertUnitCoverageComment,
} from "./unit-coverage-report.mjs";

function coverageArtifact() {
	return {
		complete: true,
		coveredStatements: 30,
		measurableStatements: 40,
		coveragePercent: 75,
		packages: [
			{ package: "pkg/ample", coveragePercent: 99, packageFloor: 10 },
			{ package: "pkg/near", coveragePercent: 80.5, packageFloor: 80 },
			{ package: "pkg/regressed", coveragePercent: 40, packageFloor: 70 },
			{ package: "pkg/ungated", coveragePercent: 12, packageFloor: null },
			// The unit lane holds unlisted packages to a 0.00 default floor.
			// Those can never regress through it.
			{ package: "pkg/lane-default", coveragePercent: 0, packageFloor: 0 },
		],
	};
}

function timingArtifact() {
	return {
		version: 1,
		complete: true,
		wallSeconds: 184.25,
		packageCount: 2,
		expectedPackageCount: 2,
		testCount: 3,
		testFailCount: 1,
		tests: [
			{ package: "pkg/alpha", test: "TestQuick", seconds: 0.25, outcome: "pass" },
			{ package: "pkg/alpha", test: "TestSlow", seconds: 40.125, outcome: "fail" },
			{ package: "pkg/beta", test: "TestMiddle", seconds: 12.5, outcome: "pass" },
		],
	};
}

test("orders gated packages by headroom ascending with violations first", () => {
	const summary = summarizeUnitCoverage(coverageArtifact(), timingArtifact());
	assert.deepEqual(
		summary.coverage.violations.map((entry) => entry.package),
		["pkg/regressed"],
	);
	assert.deepEqual(
		summary.coverage.nearFloor.map((entry) => entry.package),
		["pkg/near", "pkg/ample"],
	);
	assert.equal(summary.coverage.gatedCount, 4);
	assert.equal(summary.coverage.violationCount, 1);
});

// Regression: the first real CI run of this report accused 33 passing packages
// of breaking their floor. The producer rounds coveragePercent to one decimal
// but authors floors in basis points, so a package measured at 73.3333% against
// a 73.33 floor arrived as 73.3 and subtracted to 0.03 below. The gate passed
// while the report it publishes named those packages as violations, which is
// the one thing this report must never do.
test("judges a floor by the measured statements, not the rounded percent", () => {
	const artifact = {
		complete: true,
		coveredStatements: 11,
		measurableStatements: 15,
		coveragePercent: 73.3,
		packages: [
			// Exactly 73.3333%, which clears its floor but rounds below it.
			{
				package: "pkg/rounds-below",
				coveredStatements: 11,
				measurableStatements: 15,
				coveragePercent: 73.3,
				packageFloor: 73.33,
			},
			// Genuinely under its floor: it must stay reported.
			{
				package: "pkg/truly-under",
				coveredStatements: 7,
				measurableStatements: 10,
				coveragePercent: 70,
				packageFloor: 73.33,
			},
		],
	};
	const summary = summarizeUnitCoverage(artifact, timingArtifact());
	assert.deepEqual(
		summary.coverage.violations.map((entry) => entry.package),
		["pkg/truly-under"],
	);
	assert.equal(summary.coverage.violationCount, 1);
	assert.ok(summary.coverage.nearFloor.some((entry) => entry.package === "pkg/rounds-below"));
});

test("falls back to the reported percent when statement counts are unusable", () => {
	const artifact = {
		complete: true,
		coveredStatements: 0,
		measurableStatements: 0,
		coveragePercent: 0,
		packages: [
			// No counts at all, so the rounded percent is the only signal left.
			{ package: "pkg/countless", coveragePercent: 40, packageFloor: 70 },
			// A zero denominator must not become a division by zero.
			{
				package: "pkg/empty",
				coveredStatements: 0,
				measurableStatements: 0,
				coveragePercent: 90,
				packageFloor: 80,
			},
		],
	};
	const summary = summarizeUnitCoverage(artifact, timingArtifact());
	assert.deepEqual(
		summary.coverage.violations.map((entry) => entry.package),
		["pkg/countless"],
	);
	assert.ok(summary.coverage.nearFloor.some((entry) => entry.package === "pkg/empty"));
});

test("renders a package table ordered by headroom, not by import path", () => {
	const body = renderUnitCoverageComment(
		summarizeUnitCoverage(coverageArtifact(), timingArtifact()),
	);
	assert.ok(body.includes("## Backend Unit Coverage"));
	assert.ok(body.includes("| `pkg/regressed` | 40.00 | 70.00 | -30.00 |"));
	assert.ok(body.indexOf("### Floor violations") < body.indexOf("### Closest to their floor"));
	// pkg/near has 0.50 headroom and pkg/ample has 89.00, so headroom ordering
	// puts pkg/near first even though pkg/ample sorts first by import path.
	assert.ok(body.indexOf("`pkg/near`") < body.indexOf("`pkg/ample`"));
	// A package held to the 0.00 lane-default floor cannot be close to failing.
	assert.ok(!body.includes("`pkg/lane-default`"));
});

test("renders the slowest unit tests with real durations, ordered descending", () => {
	const summary = summarizeUnitCoverage(coverageArtifact(), timingArtifact());
	assert.deepEqual(
		summary.timing.slowest.map((entry) => entry.test),
		["TestSlow", "TestMiddle", "TestQuick"],
	);

	const body = renderUnitCoverageComment(summary);
	assert.ok(body.includes("### Slowest unit tests"));
	assert.ok(body.includes("| `TestSlow` | `pkg/alpha` | 40.125 | fail |"));
	assert.ok(body.includes("| `TestQuick` | `pkg/alpha` | 0.250 | pass |"));
	assert.ok(body.includes("- Wall-clock duration: 184.250s across 2/2 package(s)"));
});

test("states the omitted counts for every capped table instead of truncating silently", () => {
	const packages = [];
	for (let index = 0; index < UNIT_COVERAGE_PACKAGE_LIMIT + 4; index += 1) {
		packages.push({
			package: `pkg/p${String(index).padStart(2, "0")}`,
			coveragePercent: 50 + index,
			packageFloor: 50,
		});
	}
	const tests = [];
	for (let index = 0; index < UNIT_COVERAGE_SLOWEST_TEST_LIMIT + 7; index += 1) {
		tests.push({
			package: "pkg/alpha",
			test: `TestCase${String(index).padStart(2, "0")}`,
			seconds: index,
			outcome: "pass",
		});
	}

	const summary = summarizeUnitCoverage(
		{ ...coverageArtifact(), packages },
		{ ...timingArtifact(), tests },
	);
	assert.equal(summary.coverage.nearFloor.length, UNIT_COVERAGE_PACKAGE_LIMIT);
	assert.equal(summary.coverage.omitted, 4);
	assert.equal(summary.timing.slowest.length, UNIT_COVERAGE_SLOWEST_TEST_LIMIT);
	assert.equal(summary.timing.omitted, 7);

	const body = renderUnitCoverageComment(summary);
	assert.ok(body.includes("- 4 additional gated package(s) omitted."));
	assert.ok(body.includes("- 7 additional row(s) omitted."));
	assert.ok(!body.includes("`TestCase00`"));
});

test("caps the violation table while reporting the true violation count", () => {
	const packages = [];
	for (let index = 0; index < UNIT_COVERAGE_VIOLATION_LIMIT + 3; index += 1) {
		packages.push({
			package: `pkg/broken${String(index).padStart(2, "0")}`,
			coveragePercent: index,
			packageFloor: 90,
		});
	}

	const summary = summarizeUnitCoverage({ ...coverageArtifact(), packages }, null);
	assert.equal(summary.coverage.violations.length, UNIT_COVERAGE_VIOLATION_LIMIT);
	assert.equal(summary.coverage.violationCount, UNIT_COVERAGE_VIOLATION_LIMIT + 3);

	const body = renderUnitCoverageComment(summary);
	assert.ok(body.includes(`- Floor violations: ${UNIT_COVERAGE_VIOLATION_LIMIT + 3}`));
	assert.ok(body.includes("- 3 additional violation(s) omitted."));
	// The worst regression is always kept; the mildest ones are dropped.
	assert.ok(body.includes("`pkg/broken00`"));
	assert.ok(!body.includes("`pkg/broken27`"));
});

test("the job summary carries more rows than the comment and omits the marker", () => {
	const packages = [];
	for (let index = 0; index < UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT + 2; index += 1) {
		packages.push({
			package: `pkg/p${String(index).padStart(2, "0")}`,
			coveragePercent: 50 + index,
			packageFloor: 50,
		});
	}
	const tests = [];
	for (let index = 0; index < UNIT_COVERAGE_SUMMARY_SLOWEST_TEST_LIMIT + 2; index += 1) {
		tests.push({
			package: "pkg/alpha",
			test: `TestCase${String(index).padStart(2, "0")}`,
			seconds: index,
			outcome: "pass",
		});
	}

	const artifacts = [{ ...coverageArtifact(), packages }, { ...timingArtifact(), tests }];
	const jobSummary = renderUnitCoverageJobSummary(
		summarizeUnitCoverage(artifacts[0], artifacts[1], {
			violations: UNIT_COVERAGE_VIOLATION_LIMIT,
			packages: UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT,
			slowestTests: UNIT_COVERAGE_SUMMARY_SLOWEST_TEST_LIMIT,
		}),
	);

	assert.ok(!jobSummary.includes(UNIT_COVERAGE_COMMENT_MARKER));
	assert.ok(jobSummary.startsWith("## Backend Unit Coverage"));
	assert.ok(jobSummary.includes("- 2 additional gated package(s) omitted."));
	assert.ok(jobSummary.includes("- 2 additional row(s) omitted."));
	assert.equal(
		jobSummary.split("\n").filter((line) => line.startsWith("| `pkg/p")).length,
		UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT,
	);
});

test("degrades to a present body when an artifact is missing or malformed", () => {
	for (const [coverage, timing] of [
		[null, null],
		[undefined, undefined],
		["not json", 42],
		[{ packages: "not an array" }, { tests: null }],
		[coverageArtifact(), null],
	]) {
		const summary = summarizeUnitCoverage(coverage, timing);
		const body = renderUnitCoverageComment(summary);
		assert.ok(body.startsWith(UNIT_COVERAGE_COMMENT_MARKER));
		assert.ok(body.includes("## Backend Unit Coverage"));
		assert.ok(body.includes("Timing summary: unavailable"));
		assert.ok(body.includes("unit-timing-summary.json"));
	}
});

test("reports an incomplete measurement rather than presenting it as whole", () => {
	const summary = summarizeUnitCoverage(
		{ ...coverageArtifact(), complete: false, measurementReason: "lane budget expired" },
		{ ...timingArtifact(), complete: false, captureReason: "package never reported" },
	);
	const body = renderUnitCoverageComment(summary);
	assert.ok(body.includes("- Measurement status: incomplete — partial diagnostics only (lane budget expired)"));
	assert.ok(body.includes("- Capture status: incomplete — partial diagnostics only (package never reported)"));
});

test("the availability section reports readability, not mere presence", () => {
	const inputs = {
		coverage: "artifacts/coverage-summary.json",
		timing: "artifacts/unit-timing-summary.json",
		profile: "artifacts/coverage.out",
	};

	// A run with both documents parsed renders no availability section at all;
	// a healthy summary is just the report.
	const intact = renderUnitCoverageAvailability(
		summarizeUnitCoverage(coverageArtifact(), timingArtifact()),
		{ coverage: inputs.coverage, timing: inputs.timing },
	);
	assert.equal(intact, "");

	// A coverage-summary that exists on disk but does not parse is reported
	// unavailable, matching what the report above it says.
	const degraded = renderUnitCoverageAvailability(
		summarizeUnitCoverage("not json", timingArtifact()),
		inputs,
	);
	assert.ok(degraded.includes("unavailable: name=coverage-summary path=artifacts/coverage-summary.json"));
	assert.ok(degraded.includes("available: name=timing path=artifacts/unit-timing-summary.json"));
	// The profile is never opened by the renderer, so an absent one is reported
	// as absent rather than claimed present.
	assert.ok(degraded.includes("unavailable: name=coverage-profile path=artifacts/coverage.out"));
});

test("updates its own marked comment and never another report's", () => {
	const body = renderUnitCoverageComment(
		summarizeUnitCoverage(coverageArtifact(), timingArtifact()),
	);
	assert.notEqual(UNIT_COVERAGE_COMMENT_MARKER, BACKEND_LINT_COMMENT_MARKER);
	assert.notEqual(UNIT_COVERAGE_COMMENT_MARKER, FUNCTIONAL_COVERAGE_COMMENT_MARKER);
	assert.ok(!body.includes(BACKEND_LINT_COMMENT_MARKER));
	assert.ok(!body.includes(FUNCTIONAL_COVERAGE_COMMENT_MARKER));

	const comments = [
		{ id: 1, user: { login: "github-actions[bot]" }, body: `${BACKEND_LINT_COMMENT_MARKER}\nlint` },
		{
			id: 2,
			user: { login: "github-actions[bot]" },
			body: `${FUNCTIONAL_COVERAGE_COMMENT_MARKER}\nfunctional`,
		},
		{ id: 3, user: { login: "someone" }, body: `${UNIT_COVERAGE_COMMENT_MARKER}\nimpostor` },
	];

	const created = upsertUnitCoverageComment(comments, body);
	assert.equal(created.action, "create");

	const published = [
		...comments,
		{ id: 4, user: { login: "github-actions[bot]" }, body: created.body },
	];
	const updated = upsertUnitCoverageComment(published, `${body}second run`);
	assert.equal(updated.action, "update");
	assert.equal(updated.commentId, 4);
	assert.equal(updated.body, `${body}second run`);

	// A third run still targets the same single comment.
	const third = upsertUnitCoverageComment(published, `${body}third run`);
	assert.equal(third.action, "update");
	assert.equal(third.commentId, 4);
});
