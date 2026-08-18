import { readFileSync } from "node:fs";

/**
 * Shared renderer for the two halves of the backend-coverage matrix.
 *
 * The functional half shipped first and owned this logic outright. The unit
 * half renders the same three things — a headroom-ordered package table, a
 * capped slowest-tests table that always states what it dropped, and a marked
 * pull request comment that updates in place — so the ordering, capping, and
 * fail-open rules live here once and both lanes pass their own labels, markers,
 * and row caps in.
 *
 * Every function here is reporting-only and total: a missing, truncated, or
 * malformed artifact produces an explicitly unavailable summary rather than
 * throwing. Reporting must never turn a passing suite red.
 */

export const DEFAULT_COVERAGE_REPORT_LIMITS = Object.freeze({
	violations: 25,
	packages: 10,
	slowestTests: 10,
});

// Absorbs IEEE754 representation error in a headroom subtraction and nothing
// else: floors are authored to two decimals, so this is seven orders of
// magnitude below the smallest shortfall that can actually exist.
const HEADROOM_EPSILON = 1e-9;

/**
 * summarizeCoverageReport reduces a coverage-summary artifact and a timing
 * artifact to the ordered digest a report renders.
 */
export function summarizeCoverageReport(coverage, timing, limits = {}) {
	const resolved = { ...DEFAULT_COVERAGE_REPORT_LIMITS, ...limits };
	return {
		coverage: summarizeCoverageArtifact(coverage, resolved),
		timing: summarizeTimingArtifact(timing, resolved),
	};
}

function summarizeCoverageArtifact(coverage, limits) {
	if (!coverage || typeof coverage !== "object" || !Array.isArray(coverage.packages)) {
		return {
			available: false,
			violations: [],
			violationCount: 0,
			omittedViolations: 0,
			nearFloor: [],
			gatedCount: 0,
			omitted: 0,
		};
	}
	const ranked = coverage.packages
		.filter((entry) => entry && typeof entry.package === "string")
		.map((entry) => ({
			package: entry.package,
			coveragePercent: measuredCoveragePercent(entry),
			floor:
				typeof entry.packageFloor === "number" && Number.isFinite(entry.packageFloor)
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

	// A package that lands exactly on its floor meets it. Recomputing coverage
	// as a ratio means that case arrives as a value like -1.4e-14 rather than a
	// clean zero, so a bare `< 0` would report a package as failing purely
	// because 63.75 is not representable in binary. The tolerance is far below
	// the two decimals a floor is authored with, so it can only absorb
	// representation error, never a real shortfall.
	const allViolations = ranked.filter((entry) => entry.headroom < -HEADROOM_EPSILON);
	// Coverage is never negative, so a package sitting on a 0% floor cannot
	// regress through it. Both lanes hold packages without an explicit manifest
	// entry to a 0.00 default floor, so including those would crowd every
	// genuinely close package out of the table.
	const contenders = ranked.filter(
		(entry) => entry.headroom >= -HEADROOM_EPSILON && entry.floor > 0,
	);
	const violations = allViolations.slice(0, limits.violations);
	const nearFloor = contenders.slice(0, limits.packages);
	return {
		available: true,
		complete: coverage.complete !== false,
		measurementReason: textValue(coverage.measurementReason),
		coveredStatements: finiteNumber(coverage.coveredStatements),
		measurableStatements: finiteNumber(coverage.measurableStatements),
		coveragePercent: finiteNumber(coverage.coveragePercent),
		packageCount: coverage.packages.length,
		gatedCount: ranked.length,
		violationCount: allViolations.length,
		violations,
		omittedViolations: allViolations.length - violations.length,
		nearFloor,
		omitted: contenders.length - nearFloor.length,
	};
}

function summarizeTimingArtifact(timing, limits) {
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
	const shown = slowest.slice(0, limits.slowestTests);
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
 * renderCoverageReportBody renders one lane's Markdown report. When a marker is
 * supplied it is always the first line so an upsert can find the report again;
 * a job summary omits it.
 */
export function renderCoverageReportBody(summary, options = {}) {
	const heading = options.heading || "Backend Coverage";
	const coverageArtifactName = options.coverageArtifactName || "coverage-summary.json";
	const timingArtifactName = options.timingArtifactName || "timing-summary.json";
	const slowestHeading = options.slowestHeading || "Slowest top-level tests";
	const sections = [];
	if (options.marker) {
		sections.push(options.marker);
	}
	sections.push(`## ${heading}`, renderCoverageOverview(summary.coverage, coverageArtifactName));

	const violations = renderPackageTable(summary.coverage.violations);
	if (violations) {
		sections.push(
			`### Floor violations\n\n${violations}\n- ${summary.coverage.omittedViolations} additional violation(s) omitted.`,
		);
	}
	const nearFloor = renderPackageTable(summary.coverage.nearFloor);
	if (nearFloor) {
		sections.push(
			`### Closest to their floor\n\n${nearFloor}\n- ${summary.coverage.omitted} additional gated package(s) omitted.`,
		);
	}
	sections.push(renderTimingOverview(summary.timing, timingArtifactName));
	const slowest = renderSlowestTable(summary.timing.slowest);
	if (slowest) {
		sections.push(
			`### ${slowestHeading}\n\n${slowest}\n- ${summary.timing.omitted} additional row(s) omitted.`,
		);
	}
	const provenance = renderProvenance(options.metadata || {});
	if (provenance) {
		sections.push(provenance);
	}
	return `${sections.join("\n\n")}\n`;
}

function renderCoverageOverview(coverage, artifactName) {
	if (!coverage.available) {
		return `- Coverage summary: unavailable — this run published no readable \`${artifactName}\`.`;
	}
	const lines = [
		`- Coverage: ${coverage.coveragePercent.toFixed(1)}% (${coverage.coveredStatements}/${coverage.measurableStatements} statements) across ${coverage.packageCount} measured package(s)`,
		`- Gated packages: ${coverage.gatedCount}`,
		`- Floor violations: ${coverage.violationCount}`,
	];
	if (!coverage.complete) {
		lines.push(
			`- Measurement status: incomplete — partial diagnostics only${coverage.measurementReason ? ` (${coverage.measurementReason})` : ""}`,
		);
	}
	return lines.join("\n");
}

function renderTimingOverview(timing, artifactName) {
	if (!timing.available) {
		return `- Timing summary: unavailable — this run published no readable \`${artifactName}\`.`;
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
 * upsertMarkedComment finds a report's own marked bot comment and updates it,
 * so a second run on the same pull request replaces the report instead of
 * appending a duplicate. Each report passes its own marker, so two reports on
 * one thread never find, overwrite, or delete each other's comment.
 */
export function upsertMarkedComment(comments, body, options = {}) {
	const botLogin = options.botLogin || "github-actions[bot]";
	const marker = options.marker;
	if (!marker) {
		throw new Error("upsertMarkedComment requires the report's own comment marker");
	}
	const existing = (comments || []).find(
		(comment) => comment.user?.login === botLogin && comment.body?.includes(marker),
	);
	if (existing) {
		return { action: "update", commentId: existing.id, body };
	}
	return { action: "create", body };
}

/** readCoverageReportJson returns null for an absent or malformed document. */
export function readCoverageReportJson(path) {
	if (!path) {
		return null;
	}
	try {
		return JSON.parse(readFileSync(path, "utf8"));
	} catch {
		return null;
	}
}

/** coverageReportOptionValue reads a `--name value` pair out of argv. */
export function coverageReportOptionValue(args, name) {
	const index = args.indexOf(name);
	if (index === -1 || !args[index + 1]) {
		return "";
	}
	return args[index + 1];
}

/**
 * coverageReportProvenance derives the hosted head and run URL from the
 * workflow environment. Missing variables simply drop their line.
 */
export function coverageReportProvenance(env, headShaVariable) {
	return {
		headSha: (headShaVariable ? env[headShaVariable] : "") || env.GITHUB_SHA || "",
		runUrl:
			env.GITHUB_SERVER_URL && env.GITHUB_REPOSITORY && env.GITHUB_RUN_ID
				? `${env.GITHUB_SERVER_URL}/${env.GITHUB_REPOSITORY}/actions/runs/${env.GITHUB_RUN_ID}`
				: "",
	};
}

function finiteNumber(value) {
	return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

// Headroom is a subtraction between two numbers the producer records at
// different precisions: coveragePercent is rounded to one decimal, while a
// package floor carries two (it is authored in basis points and divided by
// 100). Subtracting them directly manufactures violations that do not exist --
// a package measured at 73.3333% against a 73.33 floor is comfortably above it,
// but rounds to 73.3 and reads as 0.03 below. Recomputing from the statement
// counts the same document already carries makes the comparison exact, so the
// table agrees with the gate instead of contradicting it. Falls back to the
// rounded percent when the counts are absent or unusable, because a slightly
// imprecise row is still better than dropping the package from the report.
function measuredCoveragePercent(entry) {
	const covered = entry.coveredStatements;
	const measurable = entry.measurableStatements;
	if (
		typeof covered === "number" &&
		Number.isFinite(covered) &&
		typeof measurable === "number" &&
		Number.isFinite(measurable) &&
		measurable > 0
	) {
		return (covered / measurable) * 100;
	}
	return finiteNumber(entry.coveragePercent);
}

function textValue(value) {
	return typeof value === "string" ? value.trim() : "";
}
