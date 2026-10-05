import { appendFileSync, readFileSync, writeFileSync, mkdirSync, mkdtempSync, rmSync, realpathSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { basename, dirname, join, resolve } from "node:path";

import {
	BACKEND_LINT_BASELINE_SOURCE,
	evaluateBackendLintPolicy,
} from "./backend-lint-policy.mjs";

import { tmpdir } from "node:os";

const REPORT_VERSION = 1;
const DIAGNOSTIC_PREVIEW_LIMIT = 4000;
const CLEAN_TARGET_BASELINE = 0;
const ADDED_FINDINGS_HEADER = "New unused frontend code:";
const REPORT_TARGET_STATUSES = new Set(["pass", "fail"]);
export const BACKEND_LINT_COMMENT_MARKER = "<!-- backend-lint-report -->";

function textValue(value) {
	return String(value ?? "").trim();
}

function numericValue(value, fallback = 0) {
	const number = Number(value);
	return Number.isFinite(number) ? number : fallback;
}

function normalizedStatus(value) {
	return ["pass", "passed", "success"].includes(textValue(value).toLowerCase())
		? "pass"
		: "fail";
}

function isAddedFindingsTerminator(line) {
	return line === "Baseline entries no longer reported:"
		|| line.startsWith("Current report written to ")
		|| line.startsWith("Remove the unused code ")
		|| /^LINT_VIOLATION_COUNT:\s*\d+$/.test(line);
}

function parseNamedFinding(line) {
	const match = line.match(/^\s*-\s+(.+?)\s+(export|file|type)\s+(.+?)\s*$/);
	if (!match) {
		return null;
	}
	return {
		file: match[1],
		kind: match[2],
		name: match[3],
	};
}

export function extractAddedFindings(output) {
	const lines = textValue(output).replaceAll("\r\n", "\n").replaceAll("\r", "\n").split("\n");
	const headerIndex = lines.findIndex((line) => line.trim() === ADDED_FINDINGS_HEADER);
	if (headerIndex === -1) {
		return [];
	}

	const findings = [];
	for (const rawLine of lines.slice(headerIndex + 1)) {
		const line = rawLine.trim();
		if (isAddedFindingsTerminator(line)) {
			return findings;
		}
		if (!line) {
			continue;
		}
		const finding = parseNamedFinding(line);
		if (!finding) {
			return [];
		}
		findings.push(finding);
	}

	// A complete section must have an explicit following status/count line. This
	// prevents a truncated diagnostic from being presented as a complete set.
	return [];
}

function machineReadableViolationCount(target) {
	if (Number.isSafeInteger(target?.violationCount) && target.violationCount >= 0) {
		return {
			count: target.violationCount,
			source: target.violationCountSource || "checker-report",
		};
	}

	const matches = textValue(target?.output).split(/\r?\n/).filter((line) => /^\s*LINT_VIOLATION_COUNT:/i.test(line));
	if (matches.length !== 1 || !/^\s*LINT_VIOLATION_COUNT:\s*\d+\s*$/i.test(matches[0])) {
		return null;
	}
	const count = Number(matches[0].replace(/^\s*LINT_VIOLATION_COUNT:\s*/i, ""));
	if (!Number.isSafeInteger(count) || count < 0) {
		return null;
	}
	return { count, source: "checker-marker" };
}

export function countViolations(target) {
	if (normalizedStatus(target?.status) === "pass") {
		return { count: 0, source: "successful-check" };
	}

	const measured = machineReadableViolationCount(target);
	if (measured) {
		return measured;
	}
	return { count: null, source: "unavailable" };
}

function isRecord(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function isNonNegativeSafeInteger(value) {
	return Number.isSafeInteger(value) && value >= 0;
}

function isReportTarget(value) {
	return isRecord(value)
		&& typeof value.name === "string"
		&& textValue(value.name) !== ""
		&& REPORT_TARGET_STATUSES.has(value.status)
		&& isNonNegativeSafeInteger(value.durationMillis)
		&& typeof value.output === "string";
}

function reportShapeFailure(report) {
	if (report === null || report === undefined) {
		return "the report is missing or could not be parsed";
	}
	if (
		!isRecord(report)
		|| report.version !== REPORT_VERSION
		|| !Array.isArray(report.targets)
	) {
		return "the report is malformed";
	}
	if (!report.targets.every(isReportTarget)) {
		return "the report contains malformed checker entries";
	}
	return "";
}

function combinedDiagnostic(error, log) {
	return [textValue(error), textValue(log)].filter(Boolean).join("\n");
}

function reportErrorSummary(reason, error, log) {
	const details = combinedDiagnostic(error, log);
	const reportState = reason === "the report is missing or could not be parsed"
		? "Backend Lint could not produce its report."
		: "Backend Lint produced no trustworthy checker result.";
	const diagnostic = details
		? ` Underlying diagnostic (bounded): ${preview(details, "(no underlying setup or execution diagnostic was captured)")}`
		: " No underlying setup or execution diagnostic was captured.";
	return `Backend Lint harness failure: ${reason}; zero checkers were observed. ${reportState}${diagnostic}`;
}

function harnessFailureSummary(reason, report, options = {}) {
	const diagnostic = combinedDiagnostic(options.error, options.log);
	const error = reportErrorSummary(reason, options.error, options.log);
	return {
		ok: false,
		harnessFailure: true,
		harnessFailureReason: reason,
		harnessDiagnostic: preview(diagnostic, "(no underlying setup or execution diagnostic was captured)"),
		targets: [],
		failures: [error],
		error,
		totalDurationMillis: numericValue(report?.totalDurationMillis),
		jobs: Number.isInteger(report?.jobs) ? report.jobs : null,
		baselineSource: BACKEND_LINT_BASELINE_SOURCE,
		allowances: [],
		requiredTargets: [],
	};
}

export function summarizeBackendLintReport(report, options = {}) {
	const shapeFailure = reportShapeFailure(report);
	if (shapeFailure) {
		return harnessFailureSummary(shapeFailure, report, options);
	}
	if (report.targets.length === 0) {
		return harnessFailureSummary("the report contained zero checker results", report, options);
	}

	const targets = report.targets.map((target) => {
		const status = normalizedStatus(target.status);
		const violation = countViolations(target);
		return {
			name: textValue(target.name) || "(unnamed checker)",
			status,
			violationCount: violation.count,
			violationSource: violation.source,
			durationMillis: numericValue(target.durationMillis),
			output: textValue(target.output),
			error: textValue(target.error),
			addedFindings: extractAddedFindings(target.output),
		};
	});
	const policy = evaluateBackendLintPolicy(targets);

	return {
		ok: policy.ok,
		harnessFailure: false,
		targets: policy.targets,
		failures: policy.failures,
		baselineSource: BACKEND_LINT_BASELINE_SOURCE,
		allowances: policy.allowances,
		requiredTargets: policy.requiredTargets,
		totalDurationMillis: numericValue(report.totalDurationMillis),
		jobs: Number.isInteger(report.jobs) ? report.jobs : null,
	};
}

export function formatDuration(milliseconds) {
	const duration = Math.max(0, numericValue(milliseconds));
	if (duration < 1000) {
		return `${Math.round(duration)}ms`;
	}
	return `${(duration / 1000).toFixed(2)}s`;
}

function formatCount(value) {
	return Number.isSafeInteger(value) && value >= 0 ? String(value) : "unknown";
}

function effectiveBaseline(target) {
	return Number.isSafeInteger(target?.baselineViolationCount) && target.baselineViolationCount >= 0
		? target.baselineViolationCount
		: CLEAN_TARGET_BASELINE;
}

function formatSignedDelta(current, baseline) {
	if (!Number.isSafeInteger(current) || current < 0 || !Number.isSafeInteger(baseline) || baseline < 0) {
		return "unknown";
	}
	const delta = current - baseline;
	return `${delta >= 0 ? "+" : ""}${delta}`;
}

function formatTargetComparison(target) {
	const baseline = effectiveBaseline(target);
	const current = formatCount(target.violationCount);
	const delta = formatSignedDelta(target.violationCount, baseline);
	const policy = target.policyStatus || "unknown";
	return `- ${target.name}: baseline ${baseline} -> current ${current} (delta ${delta}; ${policy})`;
}

function formatAllowanceComparison(allowance) {
	const current = formatCount(allowance.observedViolationCount);
	const delta = formatSignedDelta(allowance.observedViolationCount, allowance.baselineViolationCount);
	return `- ${allowance.name}: baseline ${allowance.baselineViolationCount} -> current ${current} (delta ${delta}; ${allowance.status})`;
}

function formatCode(value) {
	return `\`${textValue(value).replaceAll("`", "\\`")}\``;
}

function hasPositiveDelta(target) {
	const baseline = effectiveBaseline(target);
	return Number.isSafeInteger(target.violationCount)
		&& target.violationCount > baseline;
}

function formatPolicyFailure(target) {
	const lines = [formatTargetComparison(target)];
	if (hasPositiveDelta(target) && target.addedFindings?.length > 0) {
		lines.push("  Added named findings:");
		lines.push(
			...target.addedFindings.map(
				(finding) => `  - file: ${formatCode(finding.file)}; kind: ${formatCode(finding.kind)}; symbol: ${formatCode(finding.name)}`,
			),
		);
	}
	return lines;
}

export function renderBackendLintVerdict(summary) {
	if (summary.harnessFailure) {
		return [
			"### Backend Lint harness verdict: FAILED",
			"**BACKEND LINT HARNESS FAILED:** no trustworthy checker inventory was observed.",
			`- Harness cause: ${summary.harnessFailureReason}.`,
			"- Zero checkers were observed; this is not a clean report or an ordinary checker-violation result.",
		].join("\n");
	}

	const toleratedTargets = summary.targets.filter((target) => target.policyStatus === "allowed");
	const failedTargets = summary.targets.filter(
		(target) => !["clean", "allowed"].includes(target.policyStatus),
	);
	const missingTargets = (summary.allowances || []).filter((allowance) => allowance.status === "not observed");
	const verdict = summary.ok ? "PASSED" : "FAILED";
	const lines = [
		`### Backend Lint ratchet verdict: ${verdict}`,
		summary.ok
			? "**BACKEND LINT RATCHET PASSED:** every observed target is at or below baseline."
			: "**BACKEND LINT RATCHET FAILED:** one or more observed targets rose above baseline or could not be evaluated.",
	];

	if (summary.ok) {
		lines.push("", "Tolerated failing targets:");
		lines.push(
			...(toleratedTargets.length > 0 ? toleratedTargets.map(formatTargetComparison) : ["- None."]),
		);
		return lines.join("\n");
	}

	lines.push("", "Policy failures:");
	lines.push(
		...(failedTargets.length > 0 ? failedTargets.flatMap(formatPolicyFailure) : []),
		...(missingTargets.length > 0 ? missingTargets.map(formatAllowanceComparison) : []),
	);
	if (failedTargets.length === 0 && missingTargets.length === 0) {
		lines.push(...summary.failures.map((failure) => `- ${failure}`));
	}
	return lines.join("\n");
}

function preview(text, empty = "(no checker output was captured)") {
	const safe = textValue(text).replaceAll("```", "``\\`");
	if (safe.length <= DIAGNOSTIC_PREVIEW_LIMIT) {
		return safe || empty;
	}
	return `${safe.slice(0, DIAGNOSTIC_PREVIEW_LIMIT)}\n... truncated; full output is in the uploaded artifact.`;
}

function formatMarkdownCell(value) {
	return textValue(value)
		.replaceAll("|", "\\|")
		.replaceAll("\r", "")
		.replaceAll("\n", " ") || "(none)";
}

export function renderBackendLintSummary(summary) {
	const lines = [
		"## Backend Lint",
		"",
		summary.harnessFailure
			? "**Harness result:** the lint report is not trustworthy. This failed harness verdict is distinct from both a clean report and an ordinary checker violation."
			: "**Measurement only:** the raw `make lint` inventory, including any `LINT FAILED: N target(s)` line, is not the gate result. The ratchet verdict below is authoritative and controls this step's exit code.",
		...(summary.harnessFailure
			? []
			: ["Targets without a recorded allowance use an effective baseline of `0`; any positive count remains a `new failure`."]),
		"",
		renderBackendLintVerdict(summary),
		"",
		`- Result: \`${summary.ok ? "passed" : "failed"}\``,
		`- Canonical checkers observed: \`${summary.targets.length}\``,
	];
	if (summary.harnessFailure) {
		lines.push(
			`- LINT_JOBS: \`${summary.jobs ?? "unknown"}\``,
			`- Total Backend Lint wall time: \`${formatDuration(summary.totalDurationMillis)}\``,
			"",
			"### Underlying harness diagnostic (bounded)",
			"",
			"```text",
			preview(summary.harnessDiagnostic, "(no underlying setup or execution diagnostic was captured)"),
			"```",
		);
		return `${lines.join("\n")}\n`;
	}

	lines.push(
		`- Clean checkers gated: \`${summary.targets.filter((target) => target.policyStatus === "clean").length}\``,
		`- Allowed baseline debt: \`${summary.targets.filter((target) => target.policyStatus === "allowed").length}\` checker(s) within measured limits`,
		`- Baseline source: ${summary.baselineSource || "not recorded"}`,
		`- LINT_JOBS: \`${summary.jobs ?? "unknown"}\``,
		`- Total Backend Lint wall time: \`${formatDuration(summary.totalDurationMillis)}\``,
		"",
		"| Checker | Result | Baseline | Current | Delta | Wall time | Policy |",
		"| --- | --- | ---: | ---: | ---: | ---: | --- |",
	);
	for (const target of summary.targets) {
		lines.push(
			`| ${target.name} | \`${target.status}\` | ${effectiveBaseline(target)} | ${formatCount(target.violationCount)} | ${formatSignedDelta(target.violationCount, effectiveBaseline(target))} | ${formatDuration(target.durationMillis)} | ${target.policyStatus || "unknown"} |`,
		);
	}

	lines.push(
		"",
		"### Baseline allowances",
		"",
		"Allowances are capped measured debt; they do not permit new checker failures or growth.",
		"",
		"| Checker | Baseline | Observed | Delta | Status | Reason | Owner/remediation lane | Deadline | Removal condition |",
		"| --- | ---: | ---: | ---: | --- | --- | --- | --- | --- |",
	);
	for (const allowance of summary.allowances || []) {
		lines.push(
			`| ${allowance.name} | ${allowance.baselineViolationCount} | ${allowance.observedViolationCount ?? "not observed"} | ${formatSignedDelta(allowance.observedViolationCount, allowance.baselineViolationCount)} | ${allowance.status} | ${formatMarkdownCell(allowance.reason)} | ${formatMarkdownCell(allowance.ownerOrLane)} | ${allowance.deadline} | ${formatMarkdownCell(allowance.removalCondition)} |`,
		);
	}
	if (!(summary.allowances || []).length) {
		lines.push("| (none) | — | — | — | — | No baseline allowances loaded. | — | — | — |");
	}

	lines.push(
		"",
		"### No-allowance targets",
		"",
		"These targets carry no allowance and must appear in every report; any failure fails the policy on its first run.",
		"",
		"| Checker | Observed | Status | Reason | Owner/lane |",
		"| --- | ---: | --- | --- | --- |",
	);
	for (const required of summary.requiredTargets || []) {
		lines.push(
			`| ${required.name} | ${required.observedViolationCount ?? "not observed"} | ${required.status} | ${formatMarkdownCell(required.reason)} | ${formatMarkdownCell(required.ownerOrLane)} |`,
		);
	}
	if (!(summary.requiredTargets || []).length) {
		lines.push("| (none) | — | — | No no-allowance targets loaded. | — |");
	}

	const failedTargets = summary.targets.filter((target) => target.status !== "pass");
	if (failedTargets.length > 0) {
		lines.push("", "### Failed checker diagnostics (measurement only; not the ratchet verdict)", "");
		for (const target of failedTargets) {
			lines.push(
				`<details><summary>${target.name}: ${target.violationCount ?? "unknown"} reported violation(s) (${target.policyStatus || "ungated"})</summary>`,
				"",
				"```text",
				preview(target.output || target.error),
				"```",
				"",
				"</details>",
				"",
			);
		}
	}

	if (summary.error) {
		lines.push("", `> ${summary.error}`);
	}
	return `${lines.join("\n")}\n`;
}

export function renderBackendLintComment(summary, metadata = {}) {
	const lines = [BACKEND_LINT_COMMENT_MARKER, renderBackendLintSummary(summary).trim()];
	if (textValue(metadata.testedSha)) {
		lines.push(`- Hosted tested SHA: \`${textValue(metadata.testedSha)}\``);
	}
	if (textValue(metadata.runUrl)) {
		lines.push(`- Hosted run: ${textValue(metadata.runUrl)}`);
	}
	return `${lines.join("\n\n")}\n`;
}

function readReport(reportPath, logPath) {
	let log = "";
	if (logPath) {
		try {
			log = readFileSync(logPath, "utf8");
		} catch {
			log = "";
		}
	}
	try {
		return { report: JSON.parse(readFileSync(reportPath, "utf8")), log };
	} catch (error) {
		return { report: null, log, error: error.message };
	}
}

function optionValue(args, name) {
	const index = args.indexOf(name);
	if (index === -1 || !args[index + 1]) {
		throw new Error(`missing ${name} value`);
	}
	return args[index + 1];
}

// These operations consume explicit run records. GNU Make alone starts and
// schedules checks; this module never discovers sources or invokes tools.
function selectedTargets(targets) {
	if (!targets.length || targets.some((name) => !/^[a-zA-Z0-9][a-zA-Z0-9_-]*$/.test(name))) {
		throw new Error("lint selection must contain safe nonempty target names");
	}
	return [...new Set(targets)];
}

function runMetadata(directory) {
	const resolved = realpathSync(directory);
	if (dirname(resolved) !== realpathSync(tmpdir()) || !basename(resolved).startsWith("you-lint-run-")) {
		throw new Error("not an owned lint run directory");
	}
	const metadata = JSON.parse(readFileSync(join(resolved, "run.json"), "utf8"));
	if (metadata.directory !== resolved || !Number.isSafeInteger(metadata.jobs) || metadata.jobs < 1
		|| !Number.isSafeInteger(metadata.started) || metadata.started < 0
		|| JSON.stringify(selectedTargets(metadata.targets)) !== JSON.stringify(metadata.targets)) {
		throw new Error("invalid lint run metadata");
	}
	return metadata;
}

function targetPath(directory, name, suffix) {
	const metadata = runMetadata(directory);
	if (!metadata.targets.includes(name)) throw new Error(`unselected lint target: ${name}`);
	return join(metadata.directory, `${name}.${suffix}`);
}

export function beginLintRun(jobs, targets, reportPath = "") {
	// Invalidate the previous report before validation or any child can fail.
	if (reportPath) {
		mkdirSync(dirname(resolve(reportPath)), { recursive: true });
		writeFileSync(reportPath, "null\n");
	}
	if (!/^[0-9]+$/.test(jobs) || !Number.isSafeInteger(Number(jobs)) || Number(jobs) < 1) {
		throw new Error("LINT_JOBS must be a positive integer");
	}
	const selection = selectedTargets(targets);
	const directory = realpathSync(mkdtempSync(join(tmpdir(), "you-lint-run-")));
	writeFileSync(join(directory, "run.json"), JSON.stringify({ directory, jobs: Number(jobs), targets: selection, started: Date.now() }));
	return directory.replaceAll("\\", "/");
}

export function startLintTarget(directory, name) {
	const path = targetPath(directory, name, "start.json");
	mkdirSync(targetPath(directory, name, "tmp"));
	writeFileSync(path, JSON.stringify({ started: Date.now() }), { flag: "wx" });
}

export function recordLintTarget(directory, name, exitCode) {
	if (!/^[0-9]+$/.test(exitCode) || !Number.isSafeInteger(Number(exitCode))) throw new Error("invalid target exit code");
	const start = JSON.parse(readFileSync(targetPath(directory, name, "start.json"), "utf8"));
	if (!Number.isSafeInteger(start.started) || start.started < 0) throw new Error("invalid target start record");
	const output = readFileSync(targetPath(directory, name, "log"), "utf8");
	const result = { name, status: Number(exitCode) === 0 ? "pass" : "fail", durationMillis: Math.max(0, Date.now() - start.started), output };
	if (result.status === "fail") result.error = `make ${name} exited ${exitCode}`;
	const count = countViolations(result);
	if (count.count !== null) {
		result.violationCount = count.count;
		result.violationCountSource = count.source;
	}
	writeFileSync(targetPath(directory, name, "result.json"), JSON.stringify(result), { flag: "wx" });
}

export function collectLintRun(directory, targets, reportPath = "") {
	const metadata = runMetadata(directory);
	if (JSON.stringify(selectedTargets(targets)) !== JSON.stringify(metadata.targets)) throw new Error("lint selection changed during run");
	const results = metadata.targets.map((name) => {
		const result = JSON.parse(readFileSync(targetPath(directory, name, "result.json"), "utf8"));
		if (!isReportTarget(result) || result.name !== name) throw new Error(`malformed lint result: ${name}`);
		return result;
	});
	const report = { version: REPORT_VERSION, jobs: metadata.jobs, totalDurationMillis: Math.max(0, Date.now() - metadata.started), targets: results };
	if (reportPath) {
		mkdirSync(dirname(resolve(reportPath)), { recursive: true });
		writeFileSync(reportPath, JSON.stringify(report, null, 2) + "\n");
	}
	return report;
}

export function removeLintRun(directory) {
	const metadata = runMetadata(directory);
	rmSync(metadata.directory, { recursive: true });
}

function runBookkeeping(args) {
	const targets = args.includes("--") ? args.slice(args.indexOf("--") + 1) : [];
	const reportPath = args.includes("--report") ? optionValue(args, "--report") : "";
	if (args.includes("--begin-run")) {
		process.stdout.write(beginLintRun(optionValue(args, "--jobs"), targets, reportPath) + "\n");
	} else if (args.includes("--start-target")) {
		startLintTarget(optionValue(args, "--start-target"), optionValue(args, "--name"));
	} else if (args.includes("--record-target")) {
		recordLintTarget(optionValue(args, "--record-target"), optionValue(args, "--name"), optionValue(args, "--exit-code"));
	} else if (args.includes("--remove-run")) {
		removeLintRun(optionValue(args, "--remove-run"));
	} else if (args.includes("--collect-run")) {
		const report = collectLintRun(optionValue(args, "--collect-run"), targets, reportPath);
		for (const target of report.targets) {
			process.stdout.write(`===== lint target: ${target.name} =====\n${target.output}\n${target.error || ""}\n===== lint target: ${target.name}: ${target.status.toUpperCase()} =====\n`);
		}
		const failures = report.targets.filter((target) => target.status === "fail");
		process.stdout.write(failures.length ? `LINT FAILED: ${failures.length} target(s)\n` : `LINT PASSED: ${report.targets.length} target(s) completed successfully\n`);
		for (const target of failures) process.stdout.write(`  ${target.name} (rerun: make ${target.name})\n`);
		if (failures.length) process.exitCode = 1;
	} else {
		return false;
	}
	return true;
}

function runCli() {
	const args = process.argv.slice(2);
	if (runBookkeeping(args)) return;
	const reportPath = optionValue(args, "--report");
	const summaryPath = optionValue(args, "--summary");
	const commentPath = optionValue(args, "--comment");
	const input = readReport(reportPath, args.includes("--log") ? optionValue(args, "--log") : "");
	const summary = summarizeBackendLintReport(input.report, input);
	const markdown = renderBackendLintSummary(summary);
	appendFileSync(summaryPath, markdown);
	writeFileSync(commentPath, renderBackendLintComment(summary, {
		testedSha: process.env.BACKEND_LINT_TESTED_SHA,
		runUrl: process.env.GITHUB_SERVER_URL && process.env.GITHUB_REPOSITORY && process.env.GITHUB_RUN_ID
			? `${process.env.GITHUB_SERVER_URL}/${process.env.GITHUB_REPOSITORY}/actions/runs/${process.env.GITHUB_RUN_ID}`
			: "",
	}));
	process.stdout.write(markdown);
	if (!summary.ok) {
		process.exitCode = 1;
	}
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try { runCli(); } catch (error) {
		process.stderr.write(`Backend Lint harness failure: ${error.message}\n`);
		process.exitCode = 1;
	}
}
