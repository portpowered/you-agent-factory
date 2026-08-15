import { appendFileSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

import {
	BACKEND_LINT_BASELINE_SOURCE,
	evaluateBackendLintPolicy,
} from "./backend-lint-policy.mjs";

const REPORT_VERSION = 1;
const DIAGNOSTIC_PREVIEW_LIMIT = 4000;
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

function explicitViolationCount(output) {
	const currentFindings = /current findings\s*:\s*(\d+)/i.exec(output);
	if (currentFindings) {
		return Number(currentFindings[1]);
	}

	const matches = [
		...output.matchAll(
			/(?:found|reported|detected)\s+(\d+)\s+[^\n]*(?:violation|finding|error|issue)/gi,
		),
	];
	if (matches.length > 0) {
		return Number(matches.at(-1)[1]);
	}
	return null;
}

function diagnosticLineCount(output) {
	const lines = output
		.split(/\r?\n/)
		.map((line) => line.trim())
		.filter((line) => line.length > 0)
		.filter((line) => !line.startsWith("====="))
		.filter((line) => !line.startsWith("command error:"))
		.filter((line) => !/^exit status \d+$/i.test(line));
	return Math.max(1, lines.length);
}

export function countViolations(target) {
	if (normalizedStatus(target?.status) === "pass") {
		return { count: 0, source: "successful-check" };
	}

	const output = textValue(target?.output);
	const explicit = explicitViolationCount(output);
	if (explicit !== null) {
		return { count: explicit, source: "checker-output" };
	}
	return { count: diagnosticLineCount(output), source: "diagnostic-lines" };
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
		`- Result: \`${summary.ok ? "passed" : "failed"}\``,
		`- Canonical checkers observed: \`${summary.targets.length}\``,
		`- Clean checkers gated: \`${summary.targets.filter((target) => target.policyStatus === "clean").length}\``,
		`- Allowed baseline debt: \`${summary.targets.filter((target) => target.policyStatus === "allowed").length}\` checker(s) within measured limits`,
		`- Baseline source: ${summary.baselineSource || "not recorded"}`,
		`- LINT_JOBS: \`${summary.jobs ?? "unknown"}\``,
		`- Total Backend Lint wall time: \`${formatDuration(summary.totalDurationMillis)}\``,
		"",
		"| Checker | Result | Violations | Wall time | Policy |",
		"| --- | --- | ---: | ---: | --- |",
	];
	for (const target of summary.targets) {
		lines.push(
			`| ${target.name} | \`${target.status}\` | ${target.violationCount} | ${formatDuration(target.durationMillis)} | ${target.policyStatus || "unknown"} |`,
		);
	}

	lines.push(
		"",
		"### Baseline allowances",
		"",
		"Allowances are capped measured debt; they do not permit new checker failures or growth.",
		"",
		"| Checker | Baseline | Observed | Status | Reason | Owner/remediation lane | Deadline | Removal condition |",
		"| --- | ---: | ---: | --- | --- | --- | --- | --- |",
	);
	for (const allowance of summary.allowances || []) {
		lines.push(
			`| ${allowance.name} | ${allowance.baselineViolationCount} | ${allowance.observedViolationCount ?? "not observed"} | ${allowance.status} | ${formatMarkdownCell(allowance.reason)} | ${formatMarkdownCell(allowance.ownerOrLane)} | ${allowance.deadline} | ${formatMarkdownCell(allowance.removalCondition)} |`,
		);
	}
	if (!(summary.allowances || []).length) {
		lines.push("| (none) | — | — | — | No baseline allowances loaded. | — | — | — |");
	}

	const failedTargets = summary.targets.filter((target) => target.status !== "pass");
	if (failedTargets.length > 0) {
		lines.push("", "### Failed checker diagnostics", "");
		for (const target of failedTargets) {
			lines.push(
				`<details><summary>${target.name}: ${target.violationCount} reported violation(s) (${target.policyStatus || "ungated"})</summary>`,
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
	if (!summary.ok) {
		process.exitCode = 1;
	}
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	runCli();
}
