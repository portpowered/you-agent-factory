import assert from "node:assert/strict";
import test from "node:test";

import {
	BACKEND_LINT_COMMENT_MARKER,
	countViolations,
	renderBackendLintComment,
	renderBackendLintSummary,
	summarizeBackendLintReport,
} from "./backend-lint-report.mjs";

function report(overrides = {}) {
	return {
		version: 1,
		jobs: 4,
		totalDurationMillis: 12345,
		targets: [
			{
				name: "clean-check",
				status: "pass",
				durationMillis: 1200,
				output: "checker passed",
			},
			{
				name: "broken-check",
				status: "fail",
				durationMillis: 3400,
				output: "[agent-factory:broken] found 2 rule violation(s)\nfile.go:12",
			},
			{
				name: "second-broken-check",
				status: "fail",
				durationMillis: 700,
				output: "- first diagnostic\n- second diagnostic",
			},
		],
		...overrides,
	};
}

test("mixed hosted results retain the complete inventory and derive counts", () => {
	const summary = summarizeBackendLintReport(report());

	assert.equal(summary.ok, false);
	assert.deepEqual(summary.targets.map((target) => target.name), [
		"clean-check",
		"broken-check",
		"second-broken-check",
	]);
	assert.deepEqual(summary.targets.map((target) => target.violationCount), [0, 2, 2]);
	assert.match(renderBackendLintSummary(summary), /Total Backend Lint wall time: `12\.35s`/);
	assert.match(renderBackendLintSummary(summary), /\| clean-check \| `pass` \| 0 \|/);
	assert.match(renderBackendLintSummary(summary), /\| broken-check \| `fail` \| 2 \|/);
	assert.match(renderBackendLintSummary(summary), /Failed checker diagnostics/);
});

test("a successful checker always reports zero violations", () => {
	assert.deepEqual(
		countViolations({ status: "success", output: "found 99 stale words" }),
		{ count: 0, source: "successful-check" },
	);
});

test("missing or malformed hosted reports fail closed", () => {
	const summary = summarizeBackendLintReport(null, {
		error: "ENOENT",
		log: "make lint could not start",
	});

	assert.equal(summary.ok, false);
	assert.equal(summary.targets.length, 0);
	assert.match(renderBackendLintSummary(summary), /could not produce its report/);
});

test("PR publication includes a stable marker and hosted identity", () => {
	const comment = renderBackendLintComment(summarizeBackendLintReport(report()), {
		headSha: "abc123",
		runUrl: "https://github.com/example/repo/actions/runs/42",
	});

	assert.match(comment, new RegExp(BACKEND_LINT_COMMENT_MARKER.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
	assert.match(comment, /Hosted head: `abc123`/);
	assert.match(comment, /actions\/runs\/42/);
});
