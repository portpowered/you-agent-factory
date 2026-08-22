import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { BACKEND_LINT_ALLOWANCES, BACKEND_LINT_REQUIRED_TARGETS } from "./backend-lint-policy.mjs";
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
	const names = [
		...Object.keys(BACKEND_LINT_ALLOWANCES),
		...Object.keys(BACKEND_LINT_REQUIRED_TARGETS),
	];
	return names.map((name) => ({
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
	assert.equal(summary.harnessFailure, false);
	assert.equal(summary.failures.length, 0);
	assert.equal(
		summary.targets.filter((target) => target.policyStatus === "clean").length,
		baselineTargets().length,
	);
});

test("a measured baseline failure is reported but allowed at its recorded count", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "fail",
				output: "Repository dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 562\ncurrent findings: 562",
			},
		}),
	}));
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, true);
	assert.equal(summary.targets.find((target) => target.name === "deadcode").policyStatus, "allowed");
	assert.match(markdown, /Allowed baseline debt: `1` checker\(s\) within measured limits/);
	assert.match(markdown, /\| deadcode \| 562 \| 562 \| \+0 \| allowed \|/);
});

test("a measured failure below its baseline remains tolerated with a negative delta", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "fail",
				output: "LINT_VIOLATION_COUNT: 561",
			},
		}),
	}));

	assert.equal(summary.ok, true);
	assert.match(
		renderBackendLintVerdict(summary),
		/deadcode: baseline 562 -> current 561 \(delta -1; allowed\)/,
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
	assert.equal(summary.harnessFailure, false);
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
				output: "LINT_VIOLATION_COUNT: 562",
			},
		}),
	}));
	const verdict = renderBackendLintVerdict(summary);

	assert.equal(summary.ok, true);
	assert.match(verdict, /BACKEND LINT RATCHET PASSED/);
	assert.match(verdict, /every observed target is at or below baseline/);
	assert.match(verdict, /deadcode: baseline 562 -> current 562 \(delta \+0; allowed\)/);
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
			"packaged-factory-consumption-check": {
				status: "fail",
				output: "Report{MissingPackages:[]string{\"pkg/a\", \"pkg/b\"}}",
			},
		}),
	}));

	assert.equal(summary.ok, false);
	assert.equal(summary.targets.find((target) => target.name === "packaged-factory-consumption-check").violationCount, null);
	assert.match(summary.failures.join("\n"), /without a reliable machine-readable violation count/);
	assert.match(renderBackendLintVerdict(summary), /packaged-factory-consumption-check: baseline 1 -> current unknown \(delta unknown; unmeasured\)/);
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
		/deadcode: baseline 562 -> current unknown \(delta unknown; not observed\)/,
	);
});

test("structured finding growth exceeds an allowance even on one diagnostic line", () => {
	const structuredTarget = (count) => ({
		status: "fail",
		output: `inventory report: Report{MissingPackages:[]string{${Array.from({ length: count }, (_, index) => `\"finding-${index}\"`).join(", ")}}}\nLINT_VIOLATION_COUNT: ${count}`,
	});
	const baseline = summarizeBackendLintReport(report({
		targets: baselineTargets({ "packaged-factory-consumption-check": structuredTarget(1) }),
	}));
	const grown = summarizeBackendLintReport(report({
		targets: baselineTargets({ "packaged-factory-consumption-check": structuredTarget(2) }),
	}));

	assert.equal(baseline.ok, true);
	assert.equal(grown.ok, false);
	assert.match(grown.failures.join("\n"), /packaged-factory-consumption-check reported 2 violation\(s\), exceeding its baseline allowance of 1/);
});

test("ownership-inventory-check has no allowance left to absorb a regression", () => {
	assert.equal(BACKEND_LINT_ALLOWANCES["ownership-inventory-check"], undefined);

	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("ownership-inventory-check", "inventory drift\nLINT_VIOLATION_COUNT: 1"),
		],
	}));

	assert.equal(summary.ok, false);
	assert.match(
		summary.failures.join("\n"),
		/ownership-inventory-check failed with 1 reported violation\(s\); no baseline allowance exists/,
	);
	assert.match(
		renderBackendLintVerdict(summary),
		/ownership-inventory-check: baseline 0 -> current 1 \(delta \+1; new failure\)/,
	);
});

test("missing or malformed hosted reports are explicit bounded harness failures", () => {
	const summary = summarizeBackendLintReport(null, {
		error: "ENOENT",
		log: `${"make lint could not start; setup detail ".repeat(200)}tail-only`,
	});
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, false);
	assert.equal(summary.harnessFailure, true);
	assert.equal(summary.targets.length, 0);
	assert.match(summary.error, /Backend Lint harness failure/);
	assert.match(summary.error, /zero checkers were observed/);
	assert.match(summary.error, /could not produce its report/);
	assert.match(markdown, /BACKEND LINT HARNESS FAILED/);
	assert.match(markdown, /Canonical checkers observed: `0`/);
	assert.match(markdown, /Underlying harness diagnostic \(bounded\)/);
	assert.match(markdown, /truncated; full output is in the uploaded artifact/);
	assert.doesNotMatch(markdown, /tail-only/);

	const malformed = summarizeBackendLintReport({ version: 1, targets: [null] }, {
		log: "lintlane wrote an invalid checker entry",
	});
	assert.equal(malformed.harnessFailure, true);
	assert.match(malformed.error, /malformed checker entries/);
	assert.match(renderBackendLintSummary(malformed), /Zero checkers were observed/);
});

test("incomplete or invalid checker records are harness failures", () => {
	const incomplete = summarizeBackendLintReport(report({
		targets: baselineTargets().map(({ name, status }) => ({ name, status })),
	}), {
		log: "lintlane emitted checker records without producer fields",
	});

	assert.equal(incomplete.ok, false);
	assert.equal(incomplete.harnessFailure, true);
	assert.equal(incomplete.targets.length, 0);
	assert.match(incomplete.error, /malformed checker entries/);
	assert.match(incomplete.error, /zero checkers were observed/);
	assert.match(renderBackendLintSummary(incomplete), /BACKEND LINT HARNESS FAILED/);
	assert.doesNotMatch(renderBackendLintSummary(incomplete), /BACKEND LINT RATCHET/);

	const invalidStatus = summarizeBackendLintReport(report({
		targets: baselineTargets({
			"service-cycle-check": { status: "unknown" },
		}),
	}));

	assert.equal(invalidStatus.harnessFailure, true);
	assert.match(invalidStatus.error, /malformed checker entries/);
});

test("a structurally valid report with zero checkers is a harness failure", () => {
	const summary = summarizeBackendLintReport(report({ targets: [] }), {
		log: "ERR_MODULE_NOT_FOUND: scripts/ci/runner-parallelism.mjs",
	});
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, false);
	assert.equal(summary.harnessFailure, true);
	assert.equal(summary.targets.length, 0);
	assert.match(summary.error, /the report contained zero checker results/);
	assert.match(summary.error, /zero checkers were observed/);
	assert.match(markdown, /BACKEND LINT HARNESS FAILED/);
	assert.match(markdown, /ERR_MODULE_NOT_FOUND/);
	assert.doesNotMatch(markdown, /BACKEND LINT RATCHET/);
	assert.doesNotMatch(markdown, /Policy failures/);
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

test("a no-allowance target is gated from its first failing run", () => {
	const targets = baselineTargets({
		"service-cycle-check": {
			status: "fail",
			output: [
				"cross-service cycle regression: minimum feedback arc weight is 43, above the recorded ceiling of 42.",
				"LINT_VIOLATION_COUNT: 1",
			].join("\n"),
		},
	});
	const summary = summarizeBackendLintReport(report({ targets }));
	const verdict = renderBackendLintVerdict(summary);

	assert.equal(BACKEND_LINT_ALLOWANCES["service-cycle-check"], undefined);
	assert.equal(summary.ok, false);
	assert.equal(summary.targets.find((target) => target.name === "service-cycle-check").violationCount, 1);
	assert.match(verdict, /service-cycle-check: baseline 0 -> current 1 \(delta \+1; new failure\)/);
	assert.match(
		summary.failures.join("\n"),
		/service-cycle-check failed with 1 reported violation\(s\); no baseline allowance exists/,
	);
});

test("a passing no-allowance target is measured, not classified unmeasured", () => {
	const summary = summarizeBackendLintReport(report({ targets: baselineTargets() }));
	const target = summary.targets.find((item) => item.name === "service-cycle-check");

	assert.equal(summary.ok, true);
	assert.equal(target.violationCount, 0);
	assert.equal(target.policyStatus, "clean");
	assert.match(renderBackendLintSummary(summary), /\| service-cycle-check \| `pass` \| 0 \| 0 \| \+0 \|/);
});

test("dropping a no-allowance target from the lint suite fails the policy", () => {
	const targets = baselineTargets().filter((target) => target.name !== "service-cycle-check");
	const summary = summarizeBackendLintReport(report({ targets }));

	assert.equal(summary.ok, false);
	assert.match(
		summary.failures.join("\n"),
		/service-cycle-check is gated with no allowance and must run in every lint report, but it was not observed/,
	);
	assert.match(renderBackendLintSummary(summary), /### No-allowance targets/);
});
