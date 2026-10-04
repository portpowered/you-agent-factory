import { appendFileSync, existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

// Renders the flake ledger written by gocoveragecheck (see
// cmd/gocoveragecheck/coverage_flake_retry.go) as a job-summary table and
// workflow annotations. Reporting only: it never changes the job result,
// because the test run itself already decided pass or fail.

const outcomes = new Set(["flake-recovered", "failed-after-retry", "not-retried"]);

export function parseFlakeLedger(text) {
	let ledger;
	try {
		ledger = JSON.parse(text);
	} catch (error) {
		throw new Error(`flake ledger is not valid JSON: ${error.message}`);
	}
	if (ledger?.schemaVersion !== 1) {
		throw new Error(`flake ledger schemaVersion must be 1 (got ${JSON.stringify(ledger?.schemaVersion)})`);
	}
	if (!outcomes.has(ledger.outcome)) {
		throw new Error(`flake ledger outcome is not recognized: ${JSON.stringify(ledger.outcome)}`);
	}
	if (!Array.isArray(ledger.entries)) {
		throw new Error("flake ledger entries must be an array");
	}
	return ledger;
}

function cell(value) {
	return String(value ?? "").replace(/\r?\n/g, " ").replaceAll("|", "\|").slice(0, 300);
}

export function renderFlakeSummary(ledger, { runUrl = "" } = {}) {
	const lines = ["## Functional flake ledger", ""];
	const head = ledger.headSha ? `\`${String(ledger.headSha).slice(0, 12)}\`` : "unknown head";
	if (ledger.outcome === "flake-recovered") {
		lines.push(
			`**FLAKE RECORDED.** ${ledger.entries.length} test(s) failed once and passed on a same-head retry (${head}). The job passed.`,
		);
	} else if (ledger.outcome === "failed-after-retry") {
		lines.push(`**Failed after retry.** At least one test failed twice on ${head}; the job failed.`);
	} else {
		lines.push(`**Not retried.** ${ledger.reason}. The job failed as before.`);
	}
	if (runUrl) lines.push("", `Run: ${runUrl}`);
	if (ledger.entries.length > 0) {
		lines.push("", "| Test | Package | Retry | First failure excerpt |", "| --- | --- | --- | --- |");
		for (const entry of ledger.entries) {
			lines.push(
				`| \`${cell(entry.test)}\` | \`${cell(entry.package)}\` | ${cell(entry.retryOutcome)} | ${cell(entry.firstFailureExcerpt)} |`,
			);
		}
	}
	lines.push(
		"",
		"A recorded flake is a defect to fix, not a pass to ignore: give it an owner and an expiry (docs/internal/development/ci-flake-retry.md).",
	);
	return `${lines.join("\n")}\n`;
}

export function renderFlakeAnnotations(ledger) {
	if (ledger.outcome !== "flake-recovered") return [];
	return ledger.entries.map(
		(entry) =>
			`::warning title=Flaky functional test::${entry.package} ${entry.test} failed once and passed on same-head retry`,
	);
}

function runCli(argv) {
	const index = argv.indexOf("--ledger");
	const ledgerPath = index >= 0 ? argv[index + 1] : "";
	if (!ledgerPath) throw new Error("--ledger is required");
	if (!existsSync(ledgerPath)) {
		console.log("Functional flake ledger: no test failed in this run.");
		return;
	}
	const ledger = parseFlakeLedger(readFileSync(ledgerPath, "utf8"));
	const runUrl = process.env.GITHUB_SERVER_URL && process.env.GITHUB_REPOSITORY && process.env.GITHUB_RUN_ID
		? `${process.env.GITHUB_SERVER_URL}/${process.env.GITHUB_REPOSITORY}/actions/runs/${process.env.GITHUB_RUN_ID}`
		: "";
	const summary = renderFlakeSummary(ledger, { runUrl });
	process.stdout.write(summary);
	for (const annotation of renderFlakeAnnotations(ledger)) console.log(annotation);
	if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, summary);
}

const invokedPath = process.argv[1] ? pathToFileURL(resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
	try {
		runCli(process.argv.slice(2));
	} catch (error) {
		// Reporting must not mask the real test verdict.
		console.error(`Functional flake ledger unavailable: ${error.message}`);
	}
}
