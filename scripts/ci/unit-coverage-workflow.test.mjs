import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function read(path) {
	return readFileSync(join(repositoryRoot, path), "utf8");
}

function backendCoverageJob(workflow) {
	const match = workflow.match(/\n  backend-coverage:\n([\s\S]*?)\n  backend-conformance:/);
	assert.ok(match, "backend-coverage job is missing");
	return match[1];
}

function stepSection(job, marker, nextMarker) {
	const start = job.indexOf(marker);
	assert.notEqual(start, -1, `workflow step is missing: ${marker}`);
	const end = nextMarker ? job.indexOf(nextMarker, start + marker.length) : -1;
	return job.slice(start, end === -1 ? undefined : end);
}

test("unit coverage owns the only required backend unit execution", () => {
	const workflow = read(".github/workflows/ci.yml");
	assert.doesNotMatch(workflow, /^  backend-unit-latency:/m);
	assert.doesNotMatch(workflow, /name: Backend Unit Latency/);
	assert.doesNotMatch(workflow, /backend-unit-latency-evidence/);
	assert.doesNotMatch(workflow, /needs: \[[^\]]*backend-unit-latency/);
	assert.doesNotMatch(workflow, /RUN_BACKEND_UNIT_LATENCY|BACKEND_UNIT_LATENCY_RESULT/);
});

test("unit coverage uses a shallow checkout and an explicit reusable Go cache", () => {
	const job = backendCoverageJob(read(".github/workflows/ci.yml"));
	assert.match(job, /timeout-minutes: \$\{\{ matrix\.suite == 'unit' && 5 \|\| 80 \}\}/);

	const unitCheckout = stepSection(
		job,
		"      - uses: actions/checkout@v4\n        if: matrix.suite == 'unit'",
		"      - uses: actions/checkout@v4\n        if: matrix.suite == 'functional'",
	);
	assert.match(unitCheckout, /fetch-depth: 1/);
	const functionalCheckout = stepSection(
		job,
		"      - uses: actions/checkout@v4\n        if: matrix.suite == 'functional'",
		"      - uses: actions/setup-go@v5",
	);
	assert.match(functionalCheckout, /fetch-depth: 0/);

	const moduleCache = stepSection(job, "      - name: Restore Go module cache", "      - name: Restore unit coverage Go build and test cache");
	assert.match(moduleCache, /uses: actions\/cache@v4/);
	assert.match(moduleCache, /path: ~\/go\/pkg\/mod/);
	assert.match(moduleCache, /key: go-modules-/);
	assert.match(moduleCache, /functional-modules-/);

	const buildCache = stepSection(job, "      - name: Restore unit coverage Go build and test cache", "      - name: Restore functional coverage Go build cache");
	assert.match(buildCache, /if: matrix\.suite == 'unit'/);
	assert.match(buildCache, /id: unit-go-build-cache/);
	assert.match(buildCache, /uses: actions\/cache\/restore@v4/);
	assert.match(buildCache, /path: ~\/\.cache\/go-build/);
	assert.match(buildCache, /key: unit-coverage-build-/);
	assert.match(buildCache, /hashFiles\('\.github\/workflows\/ci\.yml', 'Makefile', 'go\.mod', 'go\.sum', 'cmd\/\*\*\/\*\.go', 'internal\/\*\*\/\*\.go', 'pkg\/\*\*\/\*\.go', 'docs\/internal\/baselines\/go-unit-coverage-package-minimums\.json'\)/);
	assert.match(buildCache, /functional-coverage-build-/);

	const save = stepSection(job, "      - name: Save unit coverage Go build and test cache", "      # This existing coverage tier");
	assert.match(save, /if: always\(\) && matrix\.suite == 'unit' && steps\.unit-go-build-cache\.outputs\.cache-hit != 'true'/);
	assert.match(save, /uses: actions\/cache\/save@v4/);
	assert.match(save, /key: \$\{\{ steps\.unit-go-build-cache\.outputs\.cache-primary-key \}\}/);
});

test("unit coverage pins hosted package concurrency while local callers retain platform defaults", () => {
	const workflow = backendCoverageJob(read(".github/workflows/ci.yml"));
	const run = stepSection(workflow, "      - name: Run backend coverage", "      - name: Save unit coverage Go build and test cache");
	assert.match(run, /GO_UNIT_COVERAGE_JOBS: "4"/);

	const makefile = read("Makefile");
	assert.match(makefile, /GO_UNIT_COVERAGE_JOBS \?=/);
	assert.match(makefile, /\$\(if \$\(GO_UNIT_COVERAGE_JOBS\),-jobs \$\(GO_UNIT_COVERAGE_JOBS\),\)/);
});

test("unit coverage import carriers have stable cache identities", () => {
	const source = read("cmd/gocoveragecheck/unit_coverage_imports.go");
	assert.match(source, /const unitCoverageImportFilename = "gocoveragecheck_coverage_imports_test\.go"/);
	assert.doesNotMatch(source, /os\.CreateTemp/);
	assert.match(source, /os\.O_CREATE\|os\.O_EXCL\|os\.O_WRONLY/);
});
