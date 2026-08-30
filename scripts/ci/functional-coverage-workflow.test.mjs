import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";

const workflowPath = join(process.cwd(), ".github", "workflows", "ci.yml");

function jobSection(workflow, jobName) {
	const match = workflow.match(
		new RegExp(`\\n  ${jobName}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9-]+:\\n|\\s*$)`),
	);
	assert.ok(match, `workflow job is missing: ${jobName}`);
	return match[0];
}

function stepSection(job, marker, nextMarker) {
	const start = job.indexOf(marker);
	assert.ok(start >= 0, `workflow step is missing: ${marker}`);
	const end = job.indexOf(nextMarker, start + marker.length);
	return job.slice(start, end >= 0 ? end : job.length);
}

test("functional coverage separates module and source-sensitive build caches", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const unitSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'unit'", "      - uses: actions/setup-go@v5");
	const functionalSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'functional'", "      - name: Select functional runner parallelism");
	const functionalParallelism = stepSection(job, "      - name: Select functional runner parallelism", "      - name: Restore functional Go module cache");
	const moduleCache = stepSection(job, "      - name: Restore functional Go module cache", "      - name: Restore functional coverage Go build cache");
	const buildCache = stepSection(job, "      - name: Restore functional coverage Go build cache", "      - uses: actions/setup-node@v4");

	assert.match(unitSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(unitSetup, /cache: true/);
	assert.doesNotMatch(unitSetup, /functional-coverage-|actions\/cache@v4/);

	assert.match(functionalSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(functionalSetup, /cache: false/);
	assert.match(functionalParallelism, /functional_jobs=12/);
	assert.match(functionalParallelism, /jobs=\$functional_jobs/);

	assert.match(moduleCache, /if: matrix\.suite == 'functional'/);
	assert.match(moduleCache, /id: functional-go-module-cache/);
	assert.match(moduleCache, /uses: actions\/cache@v4/);
	assert.match(moduleCache, /path: ~\/go\/pkg\/mod/);
	assert.doesNotMatch(moduleCache, /~\/\.cache\/go-build/);
	assert.match(
		moduleCache,
		/key: functional-modules-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/,
	);

	assert.match(buildCache, /if: matrix\.suite == 'functional'/);
	assert.match(buildCache, /id: functional-go-build-cache/);
	assert.match(buildCache, /uses: actions\/cache\/restore@v4/);
	assert.match(buildCache, /path: ~\/\.cache\/go-build/);
	assert.doesNotMatch(buildCache, /~\/go\/pkg\/mod/);
	assert.match(
		buildCache,
		/key: functional-coverage-build-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-jobs-\$\{\{ steps\.functional-parallelism\.outputs\.jobs \}\}-\$\{\{ hashFiles\('\.github\/workflows\/ci\.yml', 'Makefile', 'go\.mod', 'go\.sum', 'cmd\/\*\*\/\*\.go', 'internal\/\*\*\/\*\.go', 'pkg\/\*\*\/\*\.go', 'tests\/functional\/\*\*\/\*\.go', 'tests\/functional\/functional-quarantine\.json', 'docs\/internal\/baselines\/go-functional-coverage-package-minimums\.json'\) \}\}/,
	);
	assert.ok(
		buildCache.includes(
			"restore-keys: |\n            functional-coverage-build-${{ runner.os }}-${{ runner.arch }}-go-${{ env.GO_VERSION }}-jobs-${{ steps.functional-parallelism.outputs.jobs }}-\n",
		),
		"functional build cache restore key must retain jobs and the functional build namespace",
	);
	assert.equal((job.match(/uses: actions\/cache@v4/g) ?? []).length, 1);
	assert.equal((job.match(/uses: actions\/cache\/restore@v4/g) ?? []).length, 1);
	assert.equal((job.match(/uses: actions\/cache\/save@v4/g) ?? []).length, 1);
	assert.doesNotMatch(job, /functional-coverage-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/);
});

test("functional coverage forwards restore identity and saves only non-exact build restores", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const supervisorMarker = "      - name: Run Linux functional coverage with concurrent quarantine verification";
	const saveMarker = "      - name: Save functional coverage Go build cache";
	const supervisor = stepSection(job, supervisorMarker, saveMarker);
	const save = stepSection(job, saveMarker, "      - name: Upload Linux quarantine package evidence");

	for (const [name, output] of [
		["PRIMARY_KEY", "cache-primary-key"],
		["MATCHED_KEY", "cache-matched-key"],
		["EXACT_HIT", "cache-hit"],
	]) {
		assert.ok(
			supervisor.includes(
				"FUNCTIONAL_COVERAGE_ACTION_CACHE_" +
					name +
					": ${{ steps.functional-go-build-cache.outputs." +
					output +
					" }}",
			),
			`${name} restore identity is not forwarded to the coverage step`,
		);
	}

	assert.match(save, /if: always\(\) && matrix\.suite == 'functional' && steps\.functional-go-build-cache\.outputs\.cache-hit != 'true'/);
	assert.match(save, /uses: actions\/cache\/save@v4/);
	assert.match(save, /path: ~\/\.cache\/go-build/);
	assert.match(save, /key: \$\{\{ steps\.functional-go-build-cache\.outputs\.cache-primary-key \}\}/);
});

test("functional coverage joins quarantine after concurrent execution and publishes both status paths", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const supervisorMarker = "      - name: Run Linux functional coverage with concurrent quarantine verification";
	const quarantineUploadMarker = "      - name: Upload Linux quarantine package evidence";
	const supervisor = stepSection(job, supervisorMarker, "      - name: Report functional coverage verdict");
	const quarantineUpload = stepSection(job, quarantineUploadMarker, "      # Ordinary test and coverage-gate failures");

	assert.doesNotMatch(job, /      - name: Verify quarantined package inventory on Linux/);
	assert.match(supervisor, /shell: bash/);
	assert.match(supervisor, /run: bash scripts\/ci\/run-functional-coverage-with-quarantine\.sh/);
	assert.match(supervisor, /FUNCTIONAL_QUARANTINE_EVIDENCE_RUN_URL:/);
	assert.match(supervisor, /FUNCTIONAL_QUARANTINE_EVIDENCE_OUTPUT: \.artifacts\/functional-test-viz\/quarantine-package-evidence\.txt/);
	assert.match(quarantineUpload, /if: always\(\) && matrix\.suite == 'functional'/);
	assert.match(quarantineUpload, /if-no-files-found: error/);
	assert.ok(
		job.indexOf(supervisorMarker) < job.indexOf(quarantineUploadMarker),
		"quarantine evidence must upload after the supervisor has produced it",
	);
	for (const path of [
		".artifacts/functional-test-viz/c09-critical-path/quarantine-status.txt",
		".artifacts/functional-test-viz/c09-critical-path/coverage-status.txt",
		".artifacts/functional-test-viz/c09-critical-path/critical-path-timing.txt",
	]) {
		assert.match(job, new RegExp(path.replaceAll("/", "\\/")));
	}
});
