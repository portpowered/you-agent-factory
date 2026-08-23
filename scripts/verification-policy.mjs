import { appendFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve } from "node:path";

const SUCCESS = "success";
const SKIPPED = "skipped";
const MISSING = "missing";

function textValue(value) {
	return String(value ?? "").trim();
}

function normalizedResult(value) {
	const result = textValue(value).toLowerCase();
	return result || MISSING;
}

function selected(value) {
	return textValue(value) !== "false";
}

function displayValue(value, fallback = MISSING) {
	const result = textValue(value);
	return result || fallback;
}

function formatReason(reason, fallbackReason) {
	return (
		textValue(reason) ||
		textValue(fallbackReason) ||
		"No classifier reason was returned; inspect the classification job."
	);
}

function formatMarkdownCell(value) {
	return displayValue(value, "(none)")
		.replaceAll("|", "\\|")
		.replaceAll("\r", "")
		.replaceAll("\n", " ");
}

function checkLabel(lane, check) {
	return lane.checks.length === 1 ? lane.name : `${lane.name} / ${check.name}`;
}

function displayCheckResult(lane, check) {
	const result = normalizedResult(check.result);
	const required = check.required ?? selected(lane.selected);
	if (result === MISSING && !required && check.allowMissingWhenNotRequired) {
		return SKIPPED;
	}
	return result;
}

function evaluateLane(lane, failures) {
	const shouldRun = selected(lane.selected);
	for (const check of lane.checks) {
		const result = normalizedResult(check.result);
		const required = check.required ?? shouldRun;
		if (required) {
			if (result !== SUCCESS) {
				failures.push(`${checkLabel(lane, check)} was selected but finished with ${result}.`);
			}
			continue;
		}
		if (result === SKIPPED) {
			continue;
		}
		if (result === MISSING && check.allowMissingWhenNotRequired) {
			continue;
		}
		failures.push(
			`${checkLabel(lane, check)} was not selected but returned ${result}; expected skipped.`,
		);
	}
}

export function evaluateVerificationPolicy({
	classificationResult,
	classification,
	packageWorkflowResult,
	lanes,
}) {
	const failures = [];
	const normalizedClassificationResult = normalizedResult(classificationResult);
	if (normalizedClassificationResult !== SUCCESS) {
		failures.push(
			`Classification did not complete successfully (result: ${normalizedClassificationResult}).`,
		);
	}
	if (!textValue(classification)) {
		failures.push("Classification output is missing.");
	}

	const packageResult = normalizedResult(packageWorkflowResult);
	const packageSelected = lanes.some(
		(lane) => lane.packageLane && selected(lane.selected),
	);
	if (packageSelected && packageResult !== SUCCESS) {
		failures.push(
			`Development Package was needed but finished with ${packageResult}.`,
		);
	} else if (!packageSelected && ![SUCCESS, SKIPPED].includes(packageResult)) {
		failures.push(
			`Development Package was not needed but returned ${packageResult}; expected success or skipped.`,
		);
	}

	for (const lane of lanes) {
		evaluateLane(lane, failures);
	}

	return {
		ok: failures.length === 0,
		failures,
	};
}

export function renderVerificationSummary({
	classificationResult,
	classification,
	classificationReason,
	areas,
	packageWorkflowResult,
	lanes,
	evaluation,
}) {
	const lines = [
		"## Verification policy",
		"",
		`- Classification: \`${formatMarkdownCell(classification)}\``,
		`- Areas touched: \`${formatMarkdownCell(areas, "(none reported)")}\``,
		`- Classification job: \`${formatMarkdownCell(normalizedResult(classificationResult))}\``,
		`- Development Package workflow: \`${formatMarkdownCell(normalizedResult(packageWorkflowResult))}\``,
		"",
		"### Lane decisions",
		"",
		"| Lane | Decision | Terminal result | Classifier reason |",
		"| --- | --- | --- | --- |",
	];
	for (const lane of lanes) {
		const decision = selected(lane.selected) ? "run" : "skip";
		const results = lane.checks
			.map((check) => `${check.name}: ${displayCheckResult(lane, check)}`)
			.join("; ");
		lines.push(
			`| ${formatMarkdownCell(lane.name)} | \`${decision}\` | ${formatMarkdownCell(results)} | ${formatMarkdownCell(formatReason(lane.reason, classificationReason))} |`,
		);
	}
	lines.push("", `### Policy result`, "");
	if (evaluation.ok) {
		lines.push("Verification Policy passed: classification completed safely and every selected lane succeeded.");
	} else {
		lines.push("Verification Policy failed:", "");
		for (const failure of evaluation.failures) {
			lines.push(`- ${failure}`);
		}
	}
	return `${lines.join("\n")}\n`;
}

function env(name) {
	return process.env[name] ?? "";
}

function directLane(
	name,
	selectedEnv,
	reasonEnv,
	resultEnv,
	allowMissingWhenNotRequired = false,
) {
	return {
		name,
		selected: env(selectedEnv),
		reason: env(reasonEnv),
		checks: [{ name, result: env(resultEnv), allowMissingWhenNotRequired }],
	};
}

function packageLane(name, selectedEnv, reasonEnv, resultEnv, candidateResultEnv) {
	const laneSelected = selected(env(selectedEnv));
	const candidatesSelected = selected(env("RUN_CANDIDATES"));
	return {
		name,
		selected: env(selectedEnv),
		reason: env(reasonEnv),
		packageLane: true,
		checks: [
			{
				name: "verification",
				result: env(resultEnv),
				allowMissingWhenNotRequired: true,
			},
			{
				name: "candidate",
				result: env(candidateResultEnv),
				required: laneSelected && candidatesSelected,
				allowMissingWhenNotRequired: true,
			},
		],
	};
}

function policyInputFromEnvironment() {
	return {
		classificationResult: env("CLASSIFICATION_RESULT"),
		classification: env("CLASSIFICATION"),
		classificationReason: env("CLASSIFICATION_REASON"),
		areas: env("AREAS"),
		packageWorkflowResult: env("PACKAGE_WORKFLOW_RESULT"),
		lanes: [
			directLane("Docs Reference", "RUN_DOCS", "DOCS_REASON", "DOCS_RESULT"),
			directLane("README", "RUN_README", "README_REASON", "README_RESULT"),
			{
				name: "Frontend",
				selected: env("RUN_FRONTEND"),
				reason: env("FRONTEND_REASON"),
				checks: [
					{ name: "Frontend", result: env("FRONTEND_RESULT") },
					{ name: "Frontend Component", result: env("FRONTEND_COMPONENT_RESULT") },
					{ name: "Frontend Coverage", result: env("FRONTEND_COVERAGE_RESULT") },
					{ name: "Frontend Browser", result: env("FRONTEND_BROWSER_RESULT") },
					{ name: "Frontend Storybook", result: env("FRONTEND_STORYBOOK_RESULT") },
				],
			},
			directLane("Backend", "RUN_BACKEND", "BACKEND_REASON", "BACKEND_RESULT"),
			directLane(
				"Backend Test Stability",
				"RUN_BACKEND_STABILITY",
				"BACKEND_STABILITY_REASON",
				"BACKEND_STABILITY_RESULT",
			),
			directLane(
				"Backend Unit Latency",
				"RUN_BACKEND_UNIT_LATENCY",
				"BACKEND_UNIT_LATENCY_REASON",
				"BACKEND_UNIT_LATENCY_RESULT",
			),
			directLane(
				"Backend Conformance",
				"RUN_BACKEND_CONFORMANCE",
				"BACKEND_CONFORMANCE_REASON",
				"BACKEND_CONFORMANCE_RESULT",
			),
			directLane(
				"Backend Lint",
				"RUN_BACKEND_LINT",
				"BACKEND_LINT_REASON",
				"BACKEND_LINT_RESULT",
			),
			directLane(
				"Workflow Lint",
				"RUN_WORKFLOW_LINT",
				"WORKFLOW_LINT_REASON",
				"WORKFLOW_LINT_RESULT",
			),
			directLane(
				"UI Backend Integration",
				"RUN_UI_BACKEND",
				"UI_BACKEND_REASON",
				"UI_BACKEND_RESULT",
			),
			packageLane(
				"API Package",
				"RUN_API",
				"API_REASON",
				"API_RESULT",
				"API_CANDIDATE_RESULT",
			),
			packageLane(
				"Packaged Factories Package",
				"RUN_PACKAGED",
				"PACKAGED_REASON",
				"PACKAGED_RESULT",
				"PACKAGED_CANDIDATE_RESULT",
			),
			directLane(
				"Model Providers Package",
				"RUN_PROVIDERS",
				"PROVIDERS_REASON",
				"PROVIDERS_RESULT",
				true,
			),
		],
	};
}

export function runVerificationPolicy(input, summaryPath = process.env.GITHUB_STEP_SUMMARY) {
	const evaluation = evaluateVerificationPolicy(input);
	const summary = renderVerificationSummary({ ...input, evaluation });
	if (summaryPath) {
		appendFileSync(summaryPath, summary);
	}
	if (!evaluation.ok) {
		console.error("Verification Policy failed:");
		for (const failure of evaluation.failures) {
			console.error(`- ${failure}`);
		}
	}
	return evaluation;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	const evaluation = runVerificationPolicy(policyInputFromEnvironment());
	if (!evaluation.ok) {
		process.exitCode = 1;
	}
}
