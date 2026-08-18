import assert from "node:assert/strict";
import test from "node:test";

import { BACKEND_LINT_COMMENT_MARKER } from "./backend-lint-report.mjs";
import {
	FUNCTIONAL_COVERAGE_COMMENT_MARKER,
	FUNCTIONAL_COVERAGE_PACKAGE_LIMIT,
	FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT,
	FUNCTIONAL_COVERAGE_VIOLATION_LIMIT,
	renderFunctionalCoverageComment,
	summarizeFunctionalCoverage,
	upsertFunctionalCoverageComment,
} from "./functional-coverage-comment.mjs";

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
			// The functional lane holds unlisted packages to a 0.00 default
			// floor. Those can never regress through it.
			{ package: "pkg/lane-default", coveragePercent: 0, packageFloor: 0 },
		],
	};
}

function timingArtifact() {
	return {
		version: 1,
		complete: true,
		wallSeconds: 61.5,
		packageCount: 2,
		expectedPackageCount: 2,
		testCount: 3,
		testFailCount: 1,
		tests: [
			{ package: "tests/functional/alpha", test: "TestQuick", seconds: 0.25, outcome: "pass" },
			{ package: "tests/functional/alpha", test: "TestSlow", seconds: 40.125, outcome: "fail" },
			{ package: "tests/functional/beta", test: "TestMiddle", seconds: 12.5, outcome: "pass" },
		],
	};
}

test("orders gated packages by headroom ascending with violations first", () => {
	const summary = summarizeFunctionalCoverage(coverageArtifact(), timingArtifact());
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
	assert.equal(summary.coverage.omittedViolations, 0);
	assert.equal(summary.coverage.omitted, 0);
});

test("orders the slowest observed tests by elapsed descending", () => {
	const summary = summarizeFunctionalCoverage(coverageArtifact(), timingArtifact());
	assert.deepEqual(
		summary.timing.slowest.map((entry) => entry.test),
		["TestSlow", "TestMiddle", "TestQuick"],
	);
	assert.equal(summary.timing.observed, 3);
	assert.equal(summary.timing.omitted, 0);
});

test("renders a marked body distinct from the backend lint comment", () => {
	const body = renderFunctionalCoverageComment(
		summarizeFunctionalCoverage(coverageArtifact(), timingArtifact()),
		{ headSha: "abc123", runUrl: "https://example.test/run/1" },
	);
	assert.ok(body.startsWith(FUNCTIONAL_COVERAGE_COMMENT_MARKER));
	assert.notEqual(FUNCTIONAL_COVERAGE_COMMENT_MARKER, BACKEND_LINT_COMMENT_MARKER);
	assert.ok(!body.includes(BACKEND_LINT_COMMENT_MARKER));
	assert.ok(body.includes("### Floor violations"));
	assert.ok(body.includes("| `pkg/regressed` | 40.00 | 70.00 | -30.00 |"));
	assert.ok(body.includes("### Slowest top-level tests"));
	assert.ok(body.includes("| `TestSlow` | `tests/functional/alpha` | 40.125 | fail |"));
	assert.ok(body.includes("- 0 additional row(s) omitted."));
	assert.ok(body.includes("- Hosted head: `abc123`"));
	assert.ok(body.includes("https://example.test/run/1"));
	assert.ok(body.indexOf("### Floor violations") < body.indexOf("### Closest to their floor"));
	// A package held to the 0.00 lane-default floor cannot be close to failing.
	assert.ok(!body.includes("`pkg/lane-default`"));
});

test("caps both tables and states the omitted row counts", () => {
	const packages = [];
	for (let index = 0; index < FUNCTIONAL_COVERAGE_PACKAGE_LIMIT + 4; index += 1) {
		packages.push({
			package: `pkg/p${String(index).padStart(2, "0")}`,
			coveragePercent: 50 + index,
			packageFloor: 50,
		});
	}
	const tests = [];
	for (let index = 0; index < FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT + 7; index += 1) {
		tests.push({
			package: "tests/functional/alpha",
			test: `TestCase${String(index).padStart(2, "0")}`,
			seconds: index,
			outcome: "pass",
		});
	}

	const summary = summarizeFunctionalCoverage(
		{ ...coverageArtifact(), packages },
		{ ...timingArtifact(), tests },
	);
	assert.equal(summary.coverage.nearFloor.length, FUNCTIONAL_COVERAGE_PACKAGE_LIMIT);
	assert.equal(summary.coverage.omitted, 4);
	assert.equal(summary.timing.slowest.length, FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT);
	assert.equal(summary.timing.omitted, 7);

	const body = renderFunctionalCoverageComment(summary);
	assert.ok(body.includes("- 4 additional gated package(s) omitted."));
	assert.ok(body.includes("- 7 additional row(s) omitted."));
	assert.ok(!body.includes("`TestCase00`"));
});

test("caps the violation table while reporting the true violation count", () => {
	const packages = [];
	for (let index = 0; index < FUNCTIONAL_COVERAGE_VIOLATION_LIMIT + 3; index += 1) {
		packages.push({
			package: `pkg/broken${String(index).padStart(2, "0")}`,
			coveragePercent: index,
			packageFloor: 90,
		});
	}

	const summary = summarizeFunctionalCoverage({ ...coverageArtifact(), packages }, null);
	assert.equal(summary.coverage.violations.length, FUNCTIONAL_COVERAGE_VIOLATION_LIMIT);
	assert.equal(summary.coverage.violationCount, FUNCTIONAL_COVERAGE_VIOLATION_LIMIT + 3);
	assert.equal(summary.coverage.omittedViolations, 3);

	const body = renderFunctionalCoverageComment(summary);
	assert.ok(body.includes(`- Floor violations: ${FUNCTIONAL_COVERAGE_VIOLATION_LIMIT + 3}`));
	assert.ok(body.includes("- 3 additional violation(s) omitted."));
	// The worst regression is always kept; the mildest ones are dropped.
	assert.ok(body.includes("`pkg/broken00`"));
	assert.ok(!body.includes("`pkg/broken27`"));
});

test("reports missing and malformed artifacts instead of throwing", () => {
	for (const [coverage, timing] of [
		[null, null],
		[undefined, undefined],
		["not json", 42],
		[{ packages: "not an array" }, { tests: null }],
	]) {
		const summary = summarizeFunctionalCoverage(coverage, timing);
		assert.equal(summary.coverage.available, false);
		assert.equal(summary.timing.available, false);
		const body = renderFunctionalCoverageComment(summary);
		assert.ok(body.startsWith(FUNCTIONAL_COVERAGE_COMMENT_MARKER));
		assert.ok(body.includes("Coverage summary: unavailable"));
		assert.ok(body.includes("Timing summary: unavailable"));
	}
});

test("updates its own marked comment and never the backend lint comment", () => {
	const body = renderFunctionalCoverageComment(
		summarizeFunctionalCoverage(coverageArtifact(), timingArtifact()),
	);
	const comments = [
		{ id: 1, user: { login: "github-actions[bot]" }, body: `${BACKEND_LINT_COMMENT_MARKER}\nlint` },
		{ id: 2, user: { login: "someone" }, body: `${FUNCTIONAL_COVERAGE_COMMENT_MARKER}\nimpostor` },
	];

	const created = upsertFunctionalCoverageComment(comments, body);
	assert.equal(created.action, "create");
	assert.equal(created.body, body);

	const published = [
		...comments,
		{ id: 3, user: { login: "github-actions[bot]" }, body: created.body },
	];
	const updated = upsertFunctionalCoverageComment(published, `${body}second run`);
	assert.equal(updated.action, "update");
	assert.equal(updated.commentId, 3);
	assert.equal(updated.body, `${body}second run`);

	// A second upsert against the same thread still targets one comment.
	assert.equal(
		published.filter(
			(comment) =>
				comment.user.login === "github-actions[bot]" &&
				comment.body.includes(FUNCTIONAL_COVERAGE_COMMENT_MARKER),
		).length,
		1,
	);
});
