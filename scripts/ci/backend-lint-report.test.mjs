import assert from "node:assert/strict";
import test from "node:test";

import {
	BACKEND_LINT_COMMENT_MARKER,
	countViolations,
	renderBackendLintComment,
	renderBackendLintSummary,
	summarizeBackendLintReport,
} from "./backend-lint-report.mjs";
import { BACKEND_LINT_ALLOWANCES } from "./backend-lint-policy.mjs";

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
				output: "[agent-factory:broken] found 2 rule violation(s)\nLINT_VIOLATION_COUNT: 2\nfile.go:12",
			},
			{
				name: "second-broken-check",
				status: "fail",
				durationMillis: 700,
				output: "- first diagnostic\nLINT_VIOLATION_COUNT: 2\n- second diagnostic",
			},
		],
		...overrides,
	};
}

function baselineTargets(overrides = {}) {
	return Object.entries(BACKEND_LINT_ALLOWANCES).map(([name]) => ({
		name,
		status: "pass",
		durationMillis: 100,
		output: "checker passed",
		...overrides[name],
	}));
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

test("all clean current-main checkers pass the baseline policy", () => {
	const summary = summarizeBackendLintReport(report({ targets: baselineTargets() }));

	assert.equal(summary.ok, true);
	assert.equal(summary.failures.length, 0);
	assert.equal(summary.targets.filter((target) => target.policyStatus === "clean").length, 6);
});

test("a measured baseline failure is reported but allowed at its recorded count", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			"ui-deadcode": {
				status: "fail",
				output: "Frontend dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 4\ncurrent findings: 4",
			},
		}),
	}));
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, true);
	assert.equal(summary.targets.find((target) => target.name === "ui-deadcode").policyStatus, "allowed");
	assert.match(markdown, /Allowed baseline debt: `1` checker\(s\) within measured limits/);
	assert.match(markdown, /\| ui-deadcode \| 4 \| 4 \| allowed \|/);
});

test("baseline growth fails the gate instead of being hidden by an allowance", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			"ui-deadcode": {
				status: "fail",
				output: "Frontend dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 13\ncurrent findings: 13",
			},
		}),
	}));

	assert.equal(summary.ok, false);
	assert.match(summary.failures.join("\n"), /ui-deadcode reported 13 violation\(s\), exceeding its baseline allowance of 4/);
});

test("a newly failing clean checker is gated immediately", () => {
	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			{
				name: "new-clean-check",
				status: "fail",
				durationMillis: 100,
				output: "found 1 new violation(s)\nLINT_VIOLATION_COUNT: 1",
			},
		],
	}));

	assert.equal(summary.ok, false);
	assert.match(summary.failures.join("\n"), /new-clean-check failed with 1 reported violation\(s\); no baseline allowance exists/);
});

test("a successful checker always reports zero violations", () => {
	assert.deepEqual(
		countViolations({ status: "success", output: "found 99 stale words" }),
		{ count: 0, source: "successful-check" },
	);
});

test("a failed checker without a machine-readable count fails closed", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			"ownership-inventory-check": {
				status: "fail",
				output: "Report{MissingPackages:[]string{\"pkg/a\", \"pkg/b\"}}",
			},
		}),
	}));

	assert.equal(summary.ok, false);
	assert.equal(summary.targets.find((target) => target.name === "ownership-inventory-check").violationCount, null);
	assert.match(summary.failures.join("\n"), /without a reliable machine-readable violation count/);
});

test("structured finding growth exceeds the ownership allowance even on one diagnostic line", () => {
	const ownershipTarget = (count) => ({
		status: "fail",
		output: `inventory report: Report{MissingPackages:[]string{${Array.from({ length: count }, (_, index) => `\"finding-${index}\"`).join(", ")}}}\nLINT_VIOLATION_COUNT: ${count}`,
	});
	const baseline = summarizeBackendLintReport(report({
		targets: baselineTargets({ "ownership-inventory-check": ownershipTarget(16) }),
	}));
	const grown = summarizeBackendLintReport(report({
		targets: baselineTargets({ "ownership-inventory-check": ownershipTarget(17) }),
	}));

	assert.equal(baseline.ok, true);
	assert.equal(grown.ok, false);
	assert.match(grown.failures.join("\n"), /ownership-inventory-check reported 17 violation\(s\), exceeding its baseline allowance of 16/);
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
