import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
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
// UNIT_COVERAGE_COMMENT_MARKER so no two reports ever find, overwrite, or
// delete each other's pull request comment.
export const FUNCTIONAL_COVERAGE_COMMENT_MARKER =
	"<!-- functional-coverage-report -->";

// Row caps for the pull request comment. The rendered text always states how
// many rows a cap dropped so a truncated table is never read as the whole set,
// and the body stays well inside GitHub's comment size limit.
export const FUNCTIONAL_COVERAGE_VIOLATION_LIMIT = 25;
export const FUNCTIONAL_COVERAGE_PACKAGE_LIMIT = 10;
export const FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT = 10;

const FUNCTIONAL_COVERAGE_LIMITS = {
	violations: FUNCTIONAL_COVERAGE_VIOLATION_LIMIT,
	packages: FUNCTIONAL_COVERAGE_PACKAGE_LIMIT,
	slowestTests: FUNCTIONAL_COVERAGE_SLOWEST_TEST_LIMIT,
};

const FUNCTIONAL_COVERAGE_RENDER_OPTIONS = {
	marker: FUNCTIONAL_COVERAGE_COMMENT_MARKER,
	heading: "Backend Functional Coverage",
	coverageArtifactName: "coverage-summary.json",
	timingArtifactName: "functional-timing-summary.json",
	slowestHeading: "Slowest top-level tests",
	includePackageTiming: true,
	packageTimingHeading: "Functional test package timing",
	packageLimit: FUNCTIONAL_COVERAGE_PACKAGE_LIMIT,
};

/**
 * summarizeFunctionalCoverage reduces the coverage-summary and
 * functional-timing-summary artifacts to the ordered digest the pull request
 * comment renders. Absent or malformed inputs produce an explicitly
 * unavailable summary instead of throwing: reporting must never fail the job.
 */
export function summarizeFunctionalCoverage(coverage, timing) {
	return summarizeCoverageReport(coverage, timing, FUNCTIONAL_COVERAGE_LIMITS);
}

/**
 * renderFunctionalCoverageComment renders the marked pull request comment
 * body. The marker is always the first line so the upsert can find it.
 */
export function renderFunctionalCoverageComment(summary, metadata = {}) {
	return renderCoverageReportBody(summary, { ...FUNCTIONAL_COVERAGE_RENDER_OPTIONS, metadata });
}

/**
 * upsertFunctionalCoverageComment finds this report's own marked bot comment
 * and updates it, so a second run on the same pull request replaces the
 * report instead of appending a duplicate.
 */
export function upsertFunctionalCoverageComment(comments, body, options = {}) {
	return upsertMarkedComment(comments, body, {
		marker: FUNCTIONAL_COVERAGE_COMMENT_MARKER,
		...options,
	});
}

function runCli() {
	const args = process.argv.slice(2);
	const commentPath = coverageReportOptionValue(args, "--comment");
	if (!commentPath) {
		process.stderr.write("functional coverage comment: --comment path is required; skipping.\n");
		return;
	}
	const summary = summarizeFunctionalCoverage(
		readCoverageReportJson(coverageReportOptionValue(args, "--coverage")),
		readCoverageReportJson(coverageReportOptionValue(args, "--timing")),
	);
	const body = renderFunctionalCoverageComment(
		summary,
		coverageReportProvenance(process.env, "FUNCTIONAL_COVERAGE_HEAD_SHA"),
	);
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
