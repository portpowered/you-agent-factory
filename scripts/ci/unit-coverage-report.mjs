import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import {
	coverageReportOptionValue,
	coverageReportProvenance,
	readCoverageReportJson,
	renderCoverageReportBody,
	summarizeCoverageReport,
	upsertMarkedComment,
} from "./coverage-report.mjs";

// Distinct from BACKEND_LINT_COMMENT_MARKER and from
// FUNCTIONAL_COVERAGE_COMMENT_MARKER so no two reports ever find, overwrite, or
// delete each other's pull request comment.
export const UNIT_COVERAGE_COMMENT_MARKER = "<!-- unit-coverage-report -->";

// Row caps for the pull request comment. The rendered text always states how
// many rows a cap dropped so a truncated table is never read as the whole set,
// and the body stays well inside GitHub's comment size limit.
export const UNIT_COVERAGE_VIOLATION_LIMIT = 25;
export const UNIT_COVERAGE_PACKAGE_LIMIT = 10;
export const UNIT_COVERAGE_SLOWEST_TEST_LIMIT = 10;

// The job summary has no comment size limit to respect and is the place a
// contributor goes for detail, so it carries more rows than the comment. It
// states its own omitted counts for exactly the same reason.
export const UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT = 25;
export const UNIT_COVERAGE_SUMMARY_SLOWEST_TEST_LIMIT = 20;

const UNIT_COVERAGE_COMMENT_LIMITS = {
	violations: UNIT_COVERAGE_VIOLATION_LIMIT,
	packages: UNIT_COVERAGE_PACKAGE_LIMIT,
	slowestTests: UNIT_COVERAGE_SLOWEST_TEST_LIMIT,
};

const UNIT_COVERAGE_SUMMARY_LIMITS = {
	violations: UNIT_COVERAGE_VIOLATION_LIMIT,
	packages: UNIT_COVERAGE_SUMMARY_PACKAGE_LIMIT,
	slowestTests: UNIT_COVERAGE_SUMMARY_SLOWEST_TEST_LIMIT,
};

const UNIT_COVERAGE_RENDER_OPTIONS = {
	heading: "Backend Unit Coverage",
	coverageArtifactName: "coverage-summary.json",
	timingArtifactName: "unit-timing-summary.json",
	slowestHeading: "Slowest unit tests",
	includeEffectiveConcurrency: true,
};

/**
 * summarizeUnitCoverage reduces the unit lane's coverage-summary and
 * unit-timing-summary artifacts to the ordered digest the report renders.
 * Absent or malformed inputs produce an explicitly unavailable summary instead
 * of throwing: reporting must never fail the coverage job.
 */
export function summarizeUnitCoverage(coverage, timing, limits = UNIT_COVERAGE_COMMENT_LIMITS) {
	return summarizeCoverageReport(coverage, timing, limits);
}

/**
 * renderUnitCoverageComment renders the marked pull request comment body. The
 * marker is always the first line so the upsert can find it again.
 */
export function renderUnitCoverageComment(summary, metadata = {}) {
	return renderCoverageReportBody(summary, {
		...UNIT_COVERAGE_RENDER_OPTIONS,
		marker: UNIT_COVERAGE_COMMENT_MARKER,
		metadata,
	});
}

/**
 * renderUnitCoverageJobSummary renders the $GITHUB_STEP_SUMMARY body. It omits
 * the comment marker, which is only meaningful on a pull request thread.
 */
export function renderUnitCoverageJobSummary(summary, metadata = {}) {
	return renderCoverageReportBody(summary, { ...UNIT_COVERAGE_RENDER_OPTIONS, metadata });
}

/**
 * upsertUnitCoverageComment finds this report's own marked bot comment and
 * updates it, so a second run on the same pull request replaces the report
 * instead of appending a duplicate.
 */
export function upsertUnitCoverageComment(comments, body, options = {}) {
	return upsertMarkedComment(comments, body, { marker: UNIT_COVERAGE_COMMENT_MARKER, ...options });
}

/**
 * renderUnitCoverageAvailability names each input the report depends on when
 * any of them is degraded, so a partial job summary is never read as a complete
 * measurement. It reports readability, not mere presence: a file that exists but
 * does not parse is reported unreadable, which is what the report above it says.
 *
 * A run with every input intact renders nothing, keeping a healthy summary to
 * the report itself.
 */
export function renderUnitCoverageAvailability(summary, inputs = {}) {
	const rows = [
		{
			name: "coverage-summary",
			path: inputs.coverage,
			available: summary.coverage.available,
			reason: "the lane published no readable coverage summary before it ended",
		},
		{
			name: "timing",
			path: inputs.timing,
			available: summary.timing.available,
			reason: "the lane published no readable timing summary before it ended",
		},
		{
			name: "coverage-profile",
			path: inputs.profile,
			available: Boolean(inputs.profile) && existsSync(inputs.profile),
			reason: "go test did not flush a coverage profile",
		},
	].filter((row) => Boolean(row.path));

	if (rows.every((row) => row.available)) {
		return "";
	}
	const lines = ["### Unit coverage diagnostics availability", ""];
	for (const row of rows) {
		lines.push(
			row.available
				? `available: name=${row.name} path=${row.path}`
				: `unavailable: name=${row.name} path=${row.path} reason=${row.reason}`,
		);
	}
	return `${lines.join("\n")}\n`;
}

function writeReport(path, body) {
	mkdirSync(dirname(resolve(path)), { recursive: true });
	writeFileSync(path, body);
	process.stdout.write(`unit coverage report written to ${path}\n`);
}

function runCli() {
	const args = process.argv.slice(2);
	const commentPath = coverageReportOptionValue(args, "--comment");
	const summaryPath = coverageReportOptionValue(args, "--summary");
	if (!commentPath && !summaryPath) {
		process.stderr.write("unit coverage report: --comment or --summary path is required; skipping.\n");
		return;
	}
	const inputs = {
		coverage: coverageReportOptionValue(args, "--coverage"),
		timing: coverageReportOptionValue(args, "--timing"),
		profile: coverageReportOptionValue(args, "--profile"),
	};
	const coverage = readCoverageReportJson(inputs.coverage);
	const timing = readCoverageReportJson(inputs.timing);
	const metadata = coverageReportProvenance(process.env, "UNIT_COVERAGE_HEAD_SHA");
	if (summaryPath) {
		const summary = summarizeCoverageReport(coverage, timing, UNIT_COVERAGE_SUMMARY_LIMITS);
		const availability = renderUnitCoverageAvailability(summary, inputs);
		writeReport(
			summaryPath,
			renderUnitCoverageJobSummary(summary, metadata) + (availability ? `\n${availability}` : ""),
		);
	}
	if (commentPath) {
		writeReport(
			commentPath,
			renderUnitCoverageComment(summarizeUnitCoverage(coverage, timing), metadata),
		);
	}
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try {
		runCli();
	} catch (error) {
		// Reporting is never allowed to fail the coverage job.
		process.stderr.write(`unit coverage report: ${error.message}\n`);
	}
}
