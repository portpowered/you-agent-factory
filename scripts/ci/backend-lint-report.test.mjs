import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { createServer } from "node:net";
import { once } from "node:events";
import { execPath, platform } from "node:process";
import { fileURLToPath } from "node:url";

import { BACKEND_LINT_ALLOWANCES, BACKEND_LINT_REQUIRED_TARGETS } from "./backend-lint-policy.mjs";
import {
	beginLintRun, startLintTarget, recordLintTarget, collectLintRun, removeLintRun,
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

function runReporterCli(t, reportValue, environment = {}) {
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
		{
			cwd: process.cwd(),
			encoding: "utf8",
			env: { ...process.env, ...environment },
		},
	);
	return {
		...result,
		summary: readFileSync(summaryPath, "utf8"),
		comment: readFileSync(commentPath, "utf8"),
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

test("deadcode drift is a blocking failure without a Backend Lint allowance", () => {
	assert.equal(BACKEND_LINT_ALLOWANCES.deadcode, undefined);
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: unallowlistedTarget(
				"deadcode",
				"Repository dead-code baseline drift detected\nLINT_VIOLATION_COUNT: 397\ncurrent findings: 397",
			),
		}),
	}));
	const verdict = renderBackendLintVerdict(summary);

	assert.equal(summary.ok, false);
	assert.equal(summary.targets.find((target) => target.name === "deadcode").policyStatus, "new failure");
	assert.match(verdict, /deadcode: baseline 0 -> current 397 \(delta \+397; new failure\)/);
	assert.match(summary.failures.join("\n"), /deadcode failed with 397 reported violation\(s\); no baseline allowance exists/);
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

test("a clean deadcode result is reported as a gated checker", () => {
	const summary = summarizeBackendLintReport(report({
		targets: baselineTargets({
			deadcode: {
				status: "pass",
				durationMillis: 100,
				output: "[agent-factory:deadcode] baseline matches",
			},
		}),
	}));
	const markdown = renderBackendLintSummary(summary);

	assert.equal(summary.ok, true);
	assert.match(markdown, /BACKEND LINT RATCHET PASSED/);
	assert.match(markdown, /\| deadcode \| `pass` \| 0 \| 0 \| \+0 \| .* \| clean \|/);
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
			.filter((target) => target.name !== "packaged-factory-consumption-check")
			.map(({ policyStatus, baselineViolationCount, allowance, ...target }) => target),
	}));

	assert.equal(incomplete.ok, false);
	assert.match(
		renderBackendLintVerdict(incomplete),
		/packaged-factory-consumption-check: baseline 1 -> current unknown \(delta unknown; not observed\)/,
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

test("repository-fixture-check has no allowance left to absorb a regression", () => {
	assert.equal(BACKEND_LINT_ALLOWANCES["repository-fixture-check"], undefined);

	const summary = summarizeBackendLintReport(report({
		targets: [
			...baselineTargets(),
			unallowlistedTarget("repository-fixture-check", "inventory drift\nLINT_VIOLATION_COUNT: 1"),
		],
	}));

	assert.equal(summary.ok, false);
	assert.match(
		summary.failures.join("\n"),
		/repository-fixture-check failed with 1 reported violation\(s\); no baseline allowance exists/,
	);
	assert.match(
		renderBackendLintVerdict(summary),
		/repository-fixture-check: baseline 0 -> current 1 \(delta \+1; new failure\)/,
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
		log: "lint report producer wrote an invalid checker entry",
	});
	assert.equal(malformed.harnessFailure, true);
	assert.match(malformed.error, /malformed checker entries/);
	assert.match(renderBackendLintSummary(malformed), /Zero checkers were observed/);
});

test("incomplete or invalid checker records are harness failures", () => {
	const incomplete = summarizeBackendLintReport(report({
		targets: baselineTargets().map(({ name, status }) => ({ name, status })),
	}), {
		log: "lint report producer emitted checker records without producer fields",
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
		testedSha: "abc123",
		runUrl: "https://github.com/example/repo/actions/runs/42",
	});

	assert.match(comment, new RegExp(BACKEND_LINT_COMMENT_MARKER.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
	assert.match(comment, /Hosted tested SHA: `abc123`/);
	assert.match(comment, /actions\/runs\/42/);
});

test("reporter CLI publishes the explicitly supplied tested SHA", (t) => {
	const result = runReporterCli(t, report(), {
		BACKEND_LINT_TESTED_SHA: "a".repeat(40),
	});

	assert.match(result.comment, /Hosted tested SHA: `a{40}`/);
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

test("dropping deadcode from the lint suite fails the policy", () => {
	const targets = baselineTargets().filter((target) => target.name !== "deadcode");
	const summary = summarizeBackendLintReport(report({ targets }));

	assert.equal(summary.ok, false);
	assert.match(
		summary.failures.join("\n"),
		/deadcode is gated with no allowance and must run in every lint report, but it was not observed/,
	);
	assert.match(renderBackendLintSummary(summary), /### No-allowance targets/);
});

test("shared golangci diagnostics must be observed and have no allowance", () => {
	assert.equal(BACKEND_LINT_ALLOWANCES.golangci, undefined);
	const missing = summarizeBackendLintReport(report({
		targets: baselineTargets().filter((target) => target.name !== "golangci"),
	}));
	assert.equal(missing.ok, false);
	assert.match(missing.failures.join("\n"), /golangci .*must run.*not observed/);
	const failed = summarizeBackendLintReport(report({
		targets: baselineTargets({ golangci: { status: "fail", output: "LINT_VIOLATION_COUNT: 1" } }),
	}));
	assert.equal(failed.ok, false);
	assert.match(failed.failures.join("\n"), /golangci failed.*no baseline allowance exists/);
});

// Component-isolated bookkeeping cells: explicit records, no child tools.
test("collector retains ordered outcomes, exact counts and owned cleanup", (t) => {
	const directory = beginLintRun("2", ["first", "middle", "last"]);
	t.after(() => { if (existsSync(directory)) removeLintRun(directory); });
	for (const [name, exit, output] of [["last", "2", "LINT_VIOLATION_COUNT: 3"], ["middle", "0", "ok"], ["first", "1", "LINT_VIOLATION_COUNT: 2"]]) {
		startLintTarget(directory, name);
		writeFileSync(join(directory, `${name}.log`), output);
		recordLintTarget(directory, name, exit);
	}
	const observed = collectLintRun(directory, ["first", "middle", "last"]);
	assert.deepEqual(observed.targets.map(({ name, status, violationCount }) => [name, status, violationCount]), [["first", "fail", 2], ["middle", "pass", 0], ["last", "fail", 3]]);
	assert.throws(() => startLintTarget(directory, "../outside"), /unselected/);
	removeLintRun(directory);
	assert.equal(existsSync(directory), false);
});

test("setup and incomplete or malformed records fail closed", (t) => {
	for (const jobs of ["0", "-1", "many", "", "1.5"]) assert.throws(() => beginLintRun(jobs, ["first"]), /LINT_JOBS/);
	assert.throws(() => beginLintRun("2", []), /selection/);
	assert.throws(() => beginLintRun("2", ["../outside"]), /selection/);
	const root = mkdtempSync(join(tmpdir(), "you-report-errors-"));
	t.after(() => rmSync(root, { recursive: true, force: true }));
	const reportPath = join(root, "report.json");
	writeFileSync(reportPath, JSON.stringify(report()));
	const directory = beginLintRun("2", ["first"], reportPath);
	t.after(() => removeLintRun(directory));
	assert.equal(JSON.parse(readFileSync(reportPath)), null);
	assert.throws(() => collectLintRun(directory, ["first"], reportPath), /ENOENT/);
	writeFileSync(join(directory, "first.result.json"), "{}");
	assert.throws(() => collectLintRun(directory, ["first"], reportPath), /malformed/);
	assert.equal(JSON.parse(readFileSync(reportPath)), null);
	rmSync(join(directory, "first.result.json"));
	startLintTarget(directory, "first");
	writeFileSync(join(directory, "first.log"), "ok");
	recordLintTarget(directory, "first", "0");
	mkdirSync(join(root, "unwritable"));
	assert.throws(() => collectLintRun(directory, ["first"], join(root, "unwritable")), /EISDIR|EPERM|EACCES/);
	assert.throws(() => removeLintRun(root), /owned/);
});

test("malformed or duplicate count markers never invent a measurement", () => {
	for (const output of ["", "LINT_VIOLATION_COUNT: -1", "LINT_VIOLATION_COUNT: nope", "LINT_VIOLATION_COUNT: 2\nLINT_VIOLATION_COUNT: bad", "LINT_VIOLATION_COUNT: 2\nLINT_VIOLATION_COUNT: 2"]) {
		assert.deepEqual(countViolations({ status: "fail", output }), { count: null, source: "unavailable" });
	}
});

// Maintenance-tool integration: real installed Make/Node, controlled target
// effects. TCP start/release signals prove overlap without timing assertions.
test("Make drains mixed failures with bounded overlap and isolated simultaneous runs", { timeout: 120000 }, async (t) => {
	const make = platform === "win32" ? "make.exe" : "make";
	assert.equal(spawnSync(make, ["--version"]).status, 0);
	const root = mkdtempSync(join(tmpdir(), "you-make-lint-"));
	const repository = fileURLToPath(new URL("../../", import.meta.url));
	const fixture = join(root, "targets.mk").replaceAll("\\", "/");
	const script = join(root, "target.mjs").replaceAll("\\", "/");
	writeFileSync(script, `import { connect } from "node:net";
const [name] = process.argv.slice(2);
const socket = connect(Number(process.env.LINT_TEST_PORT), "127.0.0.1", () => socket.write(JSON.stringify({run:process.env.LINT_TEST_RUN, name, temp:process.env.TMPDIR}) + "\\n"));
socket.on("data", () => { console.log(name + " diagnostic temp=" + process.env.TMPDIR); console.log("LINT_VIOLATION_COUNT: 2"); socket.end(); });
socket.on("close", () => process.exit(name === "middle" ? 0 : 7));
`);
	writeFileSync(fixture, `.PHONY: first middle last\nfirst middle last:\n\t@"${execPath.replaceAll("\\", "/")}" "${script}" $@\n`);
	const groups = new Map();
	const signals = [];
	const releases = [];
	const diagnostics = new Map();
	const active = new Map();
	const peaks = new Map();
	const children = [];
	const sockets = new Set();
	const release = (socket, signal) => {
		releases.push({ run: signal.run, name: signal.name });
		socket.write("release");
	};
	const server = createServer((socket) => {
		sockets.add(socket);
		socket.on("close", () => sockets.delete(socket));
		let input = "";
		socket.on("data", (data) => {
			input += data;
			if (!input.includes("\n")) return;
			const signal = JSON.parse(input.trim());
			socket.signal = signal;
			signals.push(signal);
			const count = (active.get(signal.run) || 0) + 1;
			active.set(signal.run, count);
			peaks.set(signal.run, Math.max(peaks.get(signal.run) || 0, count));
			socket.on("close", () => active.set(signal.run, active.get(signal.run) - 1));
			if (signal.run === "success") { release(socket, signal); return; }
			const group = groups.get(signal.run) || [];
			group.push(socket);
			groups.set(signal.run, group);
			// The first pair must both start before either is released. The
			// third can only start once Make has joined a released target.
			if (group.length === 2) { for (const peer of group) release(peer, peer.signal); }
			if (group.length === 3) release(socket, signal);
		});
	});
	t.after(async () => {
		// Kill the owned process group on POSIX: killing only Make leaves its
		// gated descendants holding output pipes open, so close never arrives.
		for (const child of children) if (child.exitCode === null && child.signalCode === null) {
			if (platform === "win32") child.kill();
			else process.kill(-child.pid, "SIGKILL");
		}
		for (const socket of sockets) socket.destroy();
		server.close();
		await Promise.all(children.map((child) => !child.closed ? once(child, "close") : Promise.resolve()));
		for (const directory of new Set(signals.map((signal) => join(signal.temp, "..")))) {
			if (existsSync(directory)) removeLintRun(directory);
		}
		rmSync(root, { recursive: true, force: true });
	});
	server.listen(0, "127.0.0.1");
	await once(server, "listening");
	const launch = (run, extra = []) => {
		const reportPath = join(root, `${run}.json`);
		const child = spawn(make, ["--no-print-directory", "lint", "LINT_TARGETS=first middle last", "LINT_JOBS=2", `LINT_REPORT_FILE=${reportPath.replaceAll("\\", "/")}`, `NODE=${execPath.replaceAll("\\", "/")}`, ...extra], {
			detached: platform !== "win32",
			cwd: repository, env: { ...process.env, MAKEFILES: fixture, LINT_TEST_PORT: String(server.address().port), LINT_TEST_RUN: run },
		});
		children.push(child);
		child.once("close", () => { child.closed = true; });
		const state = { output: "" };
		diagnostics.set(run, state);
		child.stdout.on("data", (data) => { state.output += data; });
		child.stderr.on("data", (data) => { state.output += data; });
		// A failure ceiling, not synchronization: all progress uses IPC signals.
		return new Promise((resolve, reject) => {
			const timer = setTimeout(() => {
				const logs = signals.map((signal) => {
					const path = join(signal.temp, "..", `${signal.name}.log`);
					return { run: signal.run, name: signal.name, output: existsSync(path) ? readFileSync(path, "utf8") : "not observed" };
				});
				reject(new Error(`Make phase ${run} stalled\nstarts=${JSON.stringify(signals)}\nreleases=${JSON.stringify(releases)}\nchildren=${JSON.stringify([...diagnostics])}\ntargetLogs=${JSON.stringify(logs)}`));
			}, 45000);
			child.once("error", (error) => { clearTimeout(timer); reject(error); });
			child.once("close", (code) => { clearTimeout(timer); resolve({ code, output: state.output, reportPath }); });
		});
	};
	const results = await Promise.all([launch("one"), launch("two")]);
	for (const result of results) {
		assert.notEqual(result.code, 0, result.output);
		assert.ok(existsSync(result.reportPath), result.output);
		const report = JSON.parse(readFileSync(result.reportPath, "utf8"));
		assert.deepEqual(report.targets.map(({ name, status }) => [name, status]), [["first", "fail"], ["middle", "pass"], ["last", "fail"]]);
		assert.match(result.output, /LINT FAILED: 2 target/);
		assert.match(result.output, /rerun: make first/);
		assert.match(result.output, /rerun: make last/);
		for (const target of report.targets) assert.match(target.output, new RegExp(`${target.name} diagnostic`));
	}
	assert.equal(peaks.get("one"), 2);
	assert.equal(peaks.get("two"), 2);
	assert.equal(signals.length, 6);
	assert.equal(new Set(signals.map((signal) => signal.temp)).size, 6);
	for (const signal of signals) assert.equal(existsSync(signal.temp), false, "joined run temp must be removed");
	const shellArgs = platform === "win32"
		? [`SHELL=${spawnSync(make, ["-n", "lint"], { cwd: repository, encoding: "utf8" }).stdout.match(/SHELL="([^"]+)"/)[1]}`]
		: [];
	const success = await launch("success", ["LINT_TARGETS=middle", ...shellArgs]);
	assert.equal(success.code, 0, success.output);
	assert.match(success.output, /LINT PASSED: 1 target/);
	assert.deepEqual(JSON.parse(readFileSync(success.reportPath)).targets.map(({ name, status, violationCount }) => [name, status, violationCount]), [["middle", "pass", 0]]);
	const dry = await launch("dry", ["-n"]);
	assert.equal(dry.code, 0, dry.output);
	assert.equal(existsSync(dry.reportPath), false);
	assert.equal(signals.length, 7);
	for (const invalid of ["0", "-1", "many"]) {
		const rejected = await launch(`invalid-${invalid}`, [`LINT_JOBS=${invalid}`]);
		assert.notEqual(rejected.code, 0);
		assert.match(rejected.output, /LINT_JOBS must be a positive integer/);
		assert.equal(signals.length, 7);
	}
});
