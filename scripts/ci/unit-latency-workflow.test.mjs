import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");

function readWorkflow() {
	return readFileSync(join(repositoryRoot, ".github", "workflows", "ci.yml"), "utf8");
}

function readMakefile() {
	return readFileSync(join(repositoryRoot, "Makefile"), "utf8");
}

function unitLatencyJob(workflow) {
	const match = workflow.match(/\n  backend-unit-latency:\n([\s\S]*?)\n  backend-lint:/);
	assert.ok(match, "backend-unit-latency job is missing or not before backend-lint");
	return match[1];
}

test("the unit-latency job runs exactly three pinned fresh samples and enforces the budget", () => {
	const job = unitLatencyJob(readWorkflow());

	assert.match(job, /name: Backend Unit Latency/);
	assert.match(job, /if: github\.event_name == 'pull_request' \|\| github\.event_name == 'push'/);
	assert.match(job, /runs-on: ubuntu-24\.04/);
	assert.match(job, /timeout-minutes: 20/);
	assert.match(job, /UNIT_DEFAULT_JOBS: "2"/);
	assert.match(job, /UNIT_TIMING_COMMIT: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(job, /UNIT_RUNNER_PROVIDER: github-actions/);
	assert.match(job, /UNIT_RUNNER_IMAGE: ubuntu-24\.04/);
	assert.match(job, /ref: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(job, /fetch-depth: 0/);
	assert.match(job, /go-version-file: go\.mod/);
	assert.match(job, /cache: true/);
	assert.match(job, /shell: bash/);
	assert.match(job, /sample_failure=0/);
	assert.match(job, /exit "\$sample_failure"/);

	const callMatches = [...job.matchAll(/make test-unit-fresh UNIT_DEFAULT_JOBS=2 UNIT_TIMING_OUTPUT="\.artifacts\/unit-latency\/run-\$\{ordinal\}\.v2\.json"/g)];
	assert.equal(callMatches.length, 1, "the sample helper must contain one canonical Make invocation");
	assert.deepEqual(
		[...job.matchAll(/run_sample ([123])/g)].map((match) => Number(match[1])),
		[1, 2, 3],
		"the helper must be called once per ordinal in order",
	);
	assert.equal((job.match(/set \+e/g) ?? []).length, 1, "sample execution must not retry");
	assert.match(job, /status="\$\?"/);
	for (const evidenceFile of ["stdout.log", "stderr.log", "status.txt"]) {
		assert.match(job, new RegExp(`run-\\$\\{ordinal\\}\\.${evidenceFile.replace(".", "\\.")}`));
	}
	assert.match(job, /run-\$\{ordinal\}\.v2\.json/);
	assert.match(job, /run: make test-unit-latency-budget/);
});

test("the unit-latency job retains all evidence and feeds Verification Policy", () => {
	const workflow = readWorkflow();
	const job = unitLatencyJob(workflow);
	const policy = workflow.match(/\n  verification-policy:\n([\s\S]*)$/)?.[1] ?? "";

	assert.match(job, /if: always\(\)/);
	assert.match(job, /name: backend-unit-latency-evidence/);
	assert.match(job, /path: \.artifacts\/unit-latency/);
	assert.match(job, /if-no-files-found: error/);
	assert.match(job, /retention-days: 14/);
	assert.match(policy, /needs: \[[^\]]*backend-unit-latency[^\]]*\]/);
	assert.match(policy, /RUN_BACKEND_UNIT_LATENCY: "true"/);
	assert.match(policy, /BACKEND_UNIT_LATENCY_RESULT: \$\{\{ needs\.backend-unit-latency\.result \}\}/);
});

test("the Make entrypoint keeps ordinary flags and exposes additive evidence controls", () => {
	const makefile = readMakefile();

	assert.match(makefile, /UNIT_TIMING_OUTPUT \?=/);
	assert.match(makefile, /UNIT_LATENCY_BUDGET \?= docs\/internal\/baselines\/go-unit-lane-latency-budget\.v1\.json/);
	assert.match(makefile, /UNIT_LATENCY_SAMPLES \?= \.artifacts\/unit-latency\/run-1\.v2\.json,\.artifacts\/unit-latency\/run-2\.v2\.json,\.artifacts\/unit-latency\/run-3\.v2\.json/);
	assert.match(makefile, /unitlane -jobs \$\(UNIT_DEFAULT_JOBS\) -count=1 -timeout \$\(GO_TEST_TIMEOUT\)/);
	assert.match(makefile, /test-unit-latency-budget:/);
	assert.match(makefile, /unitlanebudget -budget/);
	assert.match(makefile, /unit-latency-workflow\.test\.mjs/);
});
