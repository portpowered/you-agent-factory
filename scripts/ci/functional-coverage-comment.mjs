import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

// Distinct from BACKEND_LINT_COMMENT_MARKER so the two reports never find,
// overwrite, or delete each other's pull request comment.
export const FUNCTIONAL_COVERAGE_COMMENT_MARKER =
	"<!-- functional-coverage-report -->";

// Row caps for the pull request comment. The rendered text always states how
// many rows a cap dropped so a truncated table is never read as the whole set.
export const FUNCTIONAL_COVERAGE_PACKAGE_LIMIT = 10;
export const FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT = 10;

/**
 * summarizeFunctionalCoverage reduces the coverage-summary and
 * functional-timing-summary artifacts to the ordered digest the pull request
 * comment renders. Absent or malformed inputs produce an explicitly
 * unavailable summary instead of throwing: reporting must never fail the job.
 */
export function summarizeFunctionalCoverage(coverage, timing) {
	return {
		coverage: summarizeCoverageArtifact(coverage),
		timing: summarizeTimingArtifact(timing),
	};
}

function summarizeCoverageArtifact(coverage) {
	if (!coverage || typeof coverage !== "object" || !Array.isArray(coverage.packages)) {
		return { available: false, violations: [], nearFloor: [], gatedCount: 0, omitted: 0 };
	}
	const ranked = coverage.packages
		.filter((entry) => entry && typeof entry.package === "string")
		.map((entry) => ({
			package: entry.package,
			coveragePercent: finiteNumber(entry.coveragePercent),
			floor: typeof entry.packageFloor === "number" && Number.isFinite(entry.packageFloor)
				? entry.packageFloor
				: null,
		}))
		.filter((entry) => entry.floor !== null)
		.map((entry) => ({ ...entry, headroom: entry.coveragePercent - entry.floor }))
		.sort((left, right) =>
			left.headroom === right.headroom
				? left.package.localeCompare(right.package)
				: left.headroom - right.headroom,
		);

	const violations = ranked.filter((entry) => entry.headroom < 0);
	const passing = ranked.filter((entry) => entry.headroom >= 0);
	const nearFloor = passing.slice(0, FUNCTIONAL_COVERAGE_PACKAGE_LIMIT);
	return {
		available: true,
		complete: coverage.complete !== false,
		measurementReason: textValue(coverage.measurementReason),
		coveredStatements: finiteNumber(coverage.coveredStatements),
		measurableStatements: finiteNumber(coverage.measurableStatements),
		coveragePercent: finiteNumber(coverage.coveragePercent),
		packageCount: coverage.packages.length,
		gatedCount: ranked.length,
		violations,
		nearFloor,
		omitted: passing.length - nearFloor.length,
	};
}

function summarizeTimingArtifact(timing) {
	if (!timing || typeof timing !== "object" || !Array.isArray(timing.tests)) {
		return { available: false, slowest: [], observed: 0, omitted: 0 };
	}
	const slowest = timing.tests
		.filter((entry) => entry && typeof entry.package === "string" && typeof entry.test === "string")
		.map((entry) => ({
			package: entry.package,
			test: entry.test,
			seconds: finiteNumber(entry.seconds),
			outcome: textValue(entry.outcome) || "unknown",
		}))
		.sort((left, right) =>
			left.seconds === right.seconds
				? left.package.localeCompare(right.package) || left.test.localeCompare(right.test)
				: right.seconds - left.seconds,
		);
	const shown = slowest.slice(0, FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT);
	return {
		available: true,
		complete: timing.complete !== false,
		captureReason: textValue(timing.captureReason),
		wallSeconds: finiteNumber(timing.wallSeconds),
		packageCount: finiteNumber(timing.packageCount),
		expectedPackageCount: finiteNumber(timing.expectedPackageCount),
		testFailCount: finiteNumber(timing.testFailCount),
		slowest: shown,
		observed: slowest.length,
		omitted: slowest.length - shown.length,
	};
}

/**
 * renderFunctionalCoverageComment renders the marked pull request comment
 * body. The marker is always the first line so the upsert can find it.
 */
export function renderFunctionalCoverageComment(summary, metadata = {}) {
	const sections = [
		FUNCTIONAL_COVERAGE_COMMENT_MARKER,
		"## Backend Functional Coverage",
		renderCoverageOverview(summary.coverage),
	];
	const violations = renderPackageTable(summary.coverage.violations);
	if (violations) {
		sections.push(`### Floor violations\n\n${violations}`);
	}
	const nearFloor = renderPackageTable(summary.coverage.nearFloor);
	if (nearFloor) {
		const omitted = summary.coverage.omitted;
		sections.push(
			`### Closest to their floor\n\n${nearFloor}\n- ${omitted} additional gated package(s) omitted.`,
		);
	}
	sections.push(renderTimingOverview(summary.timing));
	const slowest = renderSlowestTable(summary.timing.slowest);
	if (slowest) {
		sections.push(
			`### Slowest top-level tests\n\n${slowest}\n- ${summary.timing.omitted} additional row(s) omitted.`,
		);
	}
	const provenance = renderProvenance(metadata);
	if (provenance) {
		sections.push(provenance);
	}
	return `${sections.join("\n\n")}\n`;
}

function renderCoverageOverview(coverage) {
	if (!coverage.available) {
		return "- Coverage summary: unavailable — this run published no readable `coverage-summary.json`.";
	}
	const lines = [
		`- Coverage: ${coverage.coveragePercent.toFixed(1)}% (${coverage.coveredStatements}/${coverage.measurableStatements} statements) across ${coverage.packageCount} measured package(s)`,
		`- Gated packages: ${coverage.gatedCount}`,
		`- Floor violations: ${coverage.violations.length}`,
	];
	if (!coverage.complete) {
		lines.push(
			`- Measurement status: incomplete — partial diagnostics only${coverage.measurementReason ? ` (${coverage.measurementReason})` : ""}`,
		);
	}
	return lines.join("\n");
}

function renderTimingOverview(timing) {
	if (!timing.available) {
		return "- Timing summary: unavailable — this run published no readable `functional-timing-summary.json`.";
	}
	const lines = [
		`- Wall-clock duration: ${timing.wallSeconds.toFixed(3)}s across ${timing.packageCount}/${timing.expectedPackageCount || timing.packageCount} package(s)`,
		`- Observed top-level tests: ${timing.observed} (failed: ${timing.testFailCount})`,
	];
	if (!timing.complete) {
		lines.push(
			`- Capture status: incomplete — partial diagnostics only${timing.captureReason ? ` (${timing.captureReason})` : ""}`,
		);
	}
	return lines.join("\n");
}

function renderPackageTable(rows) {
	if (!rows || rows.length === 0) {
		return "";
	}
	const lines = ["| Package | Coverage % | Floor | Headroom |", "| --- | ---: | ---: | ---: |"];
	for (const row of rows) {
		lines.push(
			`| \`${row.package}\` | ${row.coveragePercent.toFixed(2)} | ${row.floor.toFixed(2)} | ${row.headroom.toFixed(2)} |`,
		);
	}
	return lines.join("\n");
}

function renderSlowestTable(rows) {
	if (!rows || rows.length === 0) {
		return "";
	}
	const lines = ["| Test | Package | Elapsed (s) | Outcome |", "| --- | --- | ---: | --- |"];
	for (const row of rows) {
		lines.push(
			`| \`${row.test}\` | \`${row.package}\` | ${row.seconds.toFixed(3)} | ${row.outcome} |`,
		);
	}
	return lines.join("\n");
}

function renderProvenance(metadata) {
	const lines = [];
	if (textValue(metadata.headSha)) {
		lines.push(`- Hosted head: \`${textValue(metadata.headSha)}\``);
	}
	if (textValue(metadata.runUrl)) {
		lines.push(`- Hosted run: ${textValue(metadata.runUrl)}`);
	}
	return lines.join("\n");
}

/**
 * upsertFunctionalCoverageComment finds this report's own marked bot comment
 * and updates it, so a second run on the same pull request replaces the
 * report instead of appending a duplicate.
 */
export function upsertFunctionalCoverageComment(comments, body, options = {}) {
	const botLogin = options.botLogin || "github-actions[bot]";
	const marker = options.marker || FUNCTIONAL_COVERAGE_COMMENT_MARKER;
	const existing = (comments || []).find(
		(comment) => comment.user?.login === botLogin && comment.body?.includes(marker),
	);
	if (existing) {
		return { action: "update", commentId: existing.id, body };
	}
	return { action: "create", body };
}

function finiteNumber(value) {
	return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function textValue(value) {
	return typeof value === "string" ? value.trim() : "";
}

function readJson(path) {
	if (!path) {
		return null;
	}
	try {
		return JSON.parse(readFileSync(path, "utf8"));
	} catch {
		return null;
	}
}

function optionValue(args, name) {
	const index = args.indexOf(name);
	if (index === -1 || !args[index + 1]) {
		return "";
	}
	return args[index + 1];
}

function runCli() {
	const args = process.argv.slice(2);
	const commentPath = optionValue(args, "--comment");
	if (!commentPath) {
		process.stderr.write("functional coverage comment: --comment path is required; skipping.\n");
		return;
	}
	const summary = summarizeFunctionalCoverage(
		readJson(optionValue(args, "--coverage")),
		readJson(optionValue(args, "--timing")),
	);
	const body = renderFunctionalCoverageComment(summary, {
		headSha: process.env.FUNCTIONAL_COVERAGE_HEAD_SHA || process.env.GITHUB_SHA,
		runUrl:
			process.env.GITHUB_SERVER_URL && process.env.GITHUB_REPOSITORY && process.env.GITHUB_RUN_ID
				? `${process.env.GITHUB_SERVER_URL}/${process.env.GITHUB_REPOSITORY}/actions/runs/${process.env.GITHUB_RUN_ID}`
				: "",
	});
	writeFileSync(commentPath, body);
	process.stdout.write(`functional coverage comment written to ${commentPath}\n`);
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try {
		runCli();
	} catch (error) {
		// Reporting is never allowed to fail the coverage job.
		process.stderr.write(`functional coverage comment: ${error.message}\n`);
	}
}
