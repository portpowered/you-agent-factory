import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { BACKEND_LINT_ALLOWANCES } from "./backend-lint-policy.mjs";
import {
	BACKEND_LINT_COMMENT_MARKER,
	countViolations,
	extractAddedFindings,
	renderBackendLintComment,
	renderBackendLintSummary,
	renderBackendLintVerdict,
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

function unallowlistedTarget(name, output) {
	return {
		name,
		status: "fail",
		durationMillis: 100,
		output,
	};
}

function runReporterCli(t, reportValue) {
	const directory = mkdtempSync(join(tmpdir(), "backend-lint-report-test-"));
	t.after(() => rmSync(directory, { recursive: true, force: true }));
	const reportPath = join(directory, "report.json");
	const summaryPath = join(directory, "summary.md");
	const commentPath = join(directory, "comment.md");
	writeFileSync(reportPath, JSON.stringify(reportValue));
	const result = spawnSync(
		process.execPath,
		[
			"scripts/ci/backend-lint-report.mjs",
			"--report",
			reportPath,
			"--summary",
			summaryPath,
			"--comment",
			commentPath,
		],
		{ cwd: process.cwd(), encoding: "utf8" },
	);
	return {
		...result,
		summary: readFileSync(summaryPath, "utf8"),
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
	assert.match(renderBackendLintSummary(summary), /\| clean-check \| `pass` \| 0 \| 0 \| \+0 \|/);
	assert.match(renderBackendLintSummary(summary), /\| broken-check \| `fail` \| 0 \| 2 \| \+2 \|/);
	assert.match(renderBackendLintSummary(summary), /Failed checker diagnostics/);
});

test("all clean current-main checkers pass the baseline policy", () => {
	const summary = summarizeBackendLintReport(report({ targets: baselineTargets() }));

	assert.equal(summary.ok, true);
	assert.equal(summary.failures.length, 0);
	assert.equal(summary.targets.filter((target) => target.policyStatus === "clean").length, 5);
});

test("a measured baseline failure is reported but allowed at its recorded count", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "fail",
				output: "Repository dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 580\ncurrent findings: 580",
			},
		}),
	}));
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, true);
	assert.equal(summary.targets.find((target) => target.name === "deadcode").policyStatus, "allowed");
	assert.match(markdown, /Allowed baseline debt: `1` checker\(s\) within measured limits/);
	assert.match(markdown, /\| deadcode \| 580 \| 580 \| \+0 \| allowed \|/);
});

test("a measured failure below its baseline remains tolerated with a negative delta", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "fail",
				output: "LINT_VIOLATION_COUNT: 579",
			},
		}),
	}));

	assert.equal(summary.ok, true);
	assert.match(
		renderBackendLintVerdict(summary),
		/deadcode: baseline 580 -> current 579 \(delta -1; allowed\)/,
	);
});

test("an unallowlisted ui-deadcode failure is the authoritative failed verdict", () => {
	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ui-deadcode", "Frontend dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 1"),
		],
	}));
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, false);
	assert.match(markdown, /BACKEND LINT RATCHET FAILED/);
	assert.match(markdown, /ui-deadcode: baseline 0 -> current 1 \(delta \+1; new failure\)/);
	assert.match(summary.failures.join("\n"), /ui-deadcode failed with 1 reported violation\(s\); no baseline allowance exists/);
	assert.match(markdown, /raw `make lint` inventory.*not the gate result/);
});

test("multiple lint failures are reported independently", () => {
	const targets = baselineTargets();
	targets.push(unallowlistedTarget("ui-deadcode", "LINT_VIOLATION_COUNT: 5"));
	targets.push(unallowlistedTarget("backend-size", "LINT_VIOLATION_COUNT: 3"));
	const summary = summarizeBackendLintReport(report({ targets }));
	const verdict = renderBackendLintVerdict(summary);

	assert.equal(summary.ok, false);
	assert.match(verdict, /ui-deadcode: baseline 0 -> current 5 \(delta \+5; new failure\)/);
	assert.equal(BACKEND_LINT_ALLOWANCES["backend-size"], undefined);
	assert.match(verdict, /backend-size: baseline 0 -> current 3 \(delta \+3; new failure\)/);
});

test("an unallowlisted ui-deadcode failure reports every bounded named addition", () => {
	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ui-deadcode", [
				"Frontend dead-code baseline drift detected.",
				"New unused frontend code:",
				"- ui/src/components/unused.ts export unusedExport",
				"- ui/src/types/unused.ts type UnusedType",
				"Current report written to bin/frontend-deadcode-current.json.",
				"LINT_VIOLATION_COUNT: 2",
			].join("\n")),
		],
	}));
	const verdict = renderBackendLintVerdict(summary);

	assert.deepEqual(summary.targets.find((target) => target.name === "ui-deadcode").addedFindings, [
		{ file: "ui/src/components/unused.ts", kind: "export", name: "unusedExport" },
		{ file: "ui/src/types/unused.ts", kind: "type", name: "UnusedType" },
	]);
	assert.match(verdict, /ui-deadcode: baseline 0 -> current 2 \(delta \+2; new failure\)/);
	assert.match(verdict, /Added named findings:/);
	assert.match(verdict, /file: `ui\/src\/components\/unused\.ts`; kind: `export`; symbol: `unusedExport`/);
	assert.match(verdict, /file: `ui\/src\/types\/unused\.ts`; kind: `type`; symbol: `UnusedType`/);
});

test("a count-only rise remains actionable without invented findings", () => {
	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ui-deadcode", "Frontend dead-code baseline drift detected.\nLINT_VIOLATION_COUNT: 1"),
		],
	}));
	const verdict = renderBackendLintVerdict(summary);

	assert.deepEqual(summary.targets.find((target) => target.name === "ui-deadcode").addedFindings, []);
	assert.match(verdict, /ui-deadcode: baseline 0 -> current 1 \(delta \+1; new failure\)/);
	assert.doesNotMatch(verdict, /Added named findings/);
});

test("added-finding extraction excludes removed entries and incomplete diagnostics", () => {
	const output = [
		"New unused frontend code:",
		"- ui/src/new.ts export newExport",
		"Baseline entries no longer reported:",
		"- ui/src/old.ts export oldExport",
		"LINT_VIOLATION_COUNT: 2",
	].join("\n");
	assert.deepEqual(extractAddedFindings(output), [
		{ file: "ui/src/new.ts", kind: "export", name: "newExport" },
	]);
	assert.deepEqual(
		extractAddedFindings("New unused frontend code:\n- ui/src/new.ts export newExport"),
		[],
	);
	assert.deepEqual(
		extractAddedFindings("New unused frontend code:\n- arbitrary diagnostic\nLINT_VIOLATION_COUNT: 1"),
		[],
	);
});

test("a no-rise ratchet pass states the baseline rule and tolerated debt", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "fail",
				output: "LINT_VIOLATION_COUNT: 580",
			},
		}),
	}));
	const verdict = renderBackendLintVerdict(summary);

	assert.equal(summary.ok, true);
	assert.match(verdict, /BACKEND LINT RATCHET PASSED/);
	assert.match(verdict, /every observed target is at or below baseline/);
	assert.match(verdict, /deadcode: baseline 580 -> current 580 \(delta \+0; allowed\)/);
});

test("the reporter CLI logs the authoritative verdict and exits with that decision", (t) => {
	const passing = runReporterCli(t, report({ targets: baselineTargets() }));
	assert.equal(passing.status, 0);
	assert.match(passing.stdout, /BACKEND LINT RATCHET PASSED/);
	assert.match(passing.summary, /BACKEND LINT RATCHET PASSED/);

	const failing = runReporterCli(t, report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ui-deadcode", "LINT_VIOLATION_COUNT: 1"),
		],
	}));
	assert.equal(failing.status, 1);
	assert.match(failing.stdout, /ui-deadcode: baseline 0 -> current 1 \(delta \+1; new failure\)/);
});

test("a nonzero ui-deadcode result cannot be hidden by a removed allowance", () => {
	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ui-deadcode", "Frontend dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 13\ncurrent findings: 13"),
		],
	}));

	assert.equal(summary.ok, false);
	assert.match(summary.failures.join("\n"), /ui-deadcode failed with 13 reported violation\(s\); no baseline allowance exists/);
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
	assert.match(renderBackendLintVerdict(summary), /new-clean-check: baseline 0 -> current 1 \(delta \+1; new failure\)/);
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
	assert.match(renderBackendLintVerdict(summary), /ownership-inventory-check: baseline 16 -> current unknown \(delta unknown; unmeasured\)/);
});

test("a missing allowed target is an explicit failed ratchet condition", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets(),
	}));
	const incomplete = summarizeBackendLintReport(report({
		targets: summary.targets
			.filter((target) => target.name !== "deadcode")
			.map(({ policyStatus, baselineViolationCount, allowance, ...target }) => target),
	}));

	assert.equal(incomplete.ok, false);
	assert.match(
		renderBackendLintVerdict(incomplete),
		/deadcode: baseline 580 -> current unknown \(delta unknown; not observed\)/,
	);
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
