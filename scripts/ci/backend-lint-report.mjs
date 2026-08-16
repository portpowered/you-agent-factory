import { appendFileSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

import {
	BACKEND_LINT_BASELINE_SOURCE,
	evaluateBackendLintPolicy,
} from "./backend-lint-policy.mjs";

const REPORT_VERSION = 1;
const DIAGNOSTIC_PREVIEW_LIMIT = 4000;
const CLEAN_TARGET_BASELINE = 0;
const ADDED_FINDINGS_HEADER = "New unused frontend code:";
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

	const matches = textValue(target?.output).match(/^\s*LINT_VIOLATION_COUNT:\s*(\d+)\s*$/gim) || [];
	if (matches.length !== 1) {
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

function reportErrorSummary(error, log) {
	const details = textValue(error) || textValue(log);
	return details ? `Backend Lint could not produce its report: ${details}` : "Backend Lint did not produce a report.";
}

export function summarizeBackendLintReport(report, options = {}) {
	if (!report || report.version !== REPORT_VERSION || !Array.isArray(report.targets)) {
		return {
			ok: false,
			targets: [],
			failures: [reportErrorSummary(options.error, options.log)],
			error: reportErrorSummary(options.error, options.log),
			totalDurationMillis: 0,
			jobs: null,
			baselineSource: BACKEND_LINT_BASELINE_SOURCE,
			allowances: [],
		};
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
		targets: policy.targets,
		failures: policy.failures,
		baselineSource: BACKEND_LINT_BASELINE_SOURCE,
		allowances: policy.allowances,
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

function preview(text) {
	const safe = textValue(text).replaceAll("```", "``\\`");
	if (safe.length <= DIAGNOSTIC_PREVIEW_LIMIT) {
		return safe || "(no checker output was captured)";
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
		"**Measurement only:** the raw `make lint` inventory, including any `LINT FAILED: N target(s)` line, is not the gate result. The ratchet verdict below is authoritative and controls this step's exit code.",
		"Targets without a recorded allowance use an effective baseline of `0`; any positive count remains a `new failure`.",
		"",
		renderBackendLintVerdict(summary),
		"",
		`- Result: \`${summary.ok ? "passed" : "failed"}\``,
		`- Canonical checkers observed: \`${summary.targets.length}\``,
		`- Clean checkers gated: \`${summary.targets.filter((target) => target.policyStatus === "clean").length}\``,
		`- Allowed baseline debt: \`${summary.targets.filter((target) => target.policyStatus === "allowed").length}\` checker(s) within measured limits`,
		`- Baseline source: ${summary.baselineSource || "not recorded"}`,
		`- LINT_JOBS: \`${summary.jobs ?? "unknown"}\``,
		`- Total Backend Lint wall time: \`${formatDuration(summary.totalDurationMillis)}\``,
		"",
		"| Checker | Result | Baseline | Current | Delta | Wall time | Policy |",
		"| --- | --- | ---: | ---: | ---: | ---: | --- |",
	];
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
	if (textValue(metadata.headSha)) {
		lines.push(`- Hosted head: \`${textValue(metadata.headSha)}\``);
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

function runCli() {
	const args = process.argv.slice(2);
	const reportPath = optionValue(args, "--report");
	const summaryPath = optionValue(args, "--summary");
	const commentPath = optionValue(args, "--comment");
	const input = readReport(reportPath, args.includes("--log") ? optionValue(args, "--log") : "");
	const summary = summarizeBackendLintReport(input.report, input);
	const markdown = renderBackendLintSummary(summary);
	appendFileSync(summaryPath, markdown);
	writeFileSync(commentPath, renderBackendLintComment(summary, {
		headSha: process.env.BACKEND_LINT_HEAD_SHA || process.env.GITHUB_SHA,
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
	runCli();
}
