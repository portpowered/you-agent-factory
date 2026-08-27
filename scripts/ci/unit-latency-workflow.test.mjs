import { spawnSync } from "node:child_process";
import assert from "node:assert/strict";
import {
	existsSync,
	mkdirSync,
	mkdtempSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sampleRunner = join(repositoryRoot, "scripts", "ci", "run-unit-latency-sample.mjs");

function readWorkflow() {
	return readFileSync(join(repositoryRoot, ".github", "workflows", "ci.yml"), "utf8");
}

function readMakefile() {
	return readFileSync(join(repositoryRoot, "Makefile"), "utf8");
}

function readSampleRunner() {
	return readFileSync(sampleRunner, "utf8");
}

function unitLatencyJob(workflow) {
	const match = workflow.match(/\n  backend-unit-latency:\n([\s\S]*?)\n  backend-lint:/);
	assert.ok(match, "backend-unit-latency job is missing or not before backend-lint");
	return match[1];
}

test("the unit-latency job captures ordered reference and candidate cohorts", () => {
	const job = unitLatencyJob(readWorkflow());

	assert.match(job, /name: Backend Unit Latency/);
	assert.match(job, /if: github\.event_name == 'pull_request' \|\| github\.event_name == 'push'/);
	assert.match(job, /runs-on: ubuntu-24\.04/);
	assert.match(job, /timeout-minutes: 45/);
	assert.match(job, /UNIT_DEFAULT_JOBS: "2"/);
	assert.match(job, /UNIT_REFERENCE_COMMIT: ba8ef900ee29347295ac7657742fd1aab42f064c/);
	assert.match(job, /UNIT_CANDIDATE_COMMIT: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(job, /UNIT_RUNNER_PROVIDER: github-actions/);
	assert.match(job, /UNIT_RUNNER_IMAGE: ubuntu-24\.04/);
	assert.match(job, /ref: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(job, /fetch-depth: 0/);
	assert.match(job, /go-version-file: go\.mod/);
	assert.match(job, /cache: true/);
	assert.match(job, /ImageVersion/);
	assert.match(job, /UNIT_RUNNER_IMAGE_VERSION=\$image_version/);
	assert.match(job, /UNIT_RUNNER_CPU_MODEL=\$cpu_model/);
	assert.match(job, /git worktree add --detach/);
	assert.match(job, /prewarm_checkout reference/);
	assert.match(job, /prewarm_checkout candidate/);
	assert.match(job, /shell: bash/);
	assert.match(job, /sample_failure=0/);
	assert.match(job, /exit "\$sample_failure"/);

	assert.match(job, /make test-unit-fresh UNIT_DEFAULT_JOBS=2 UNIT_TIMING_OUTPUT="\$timing_output"/);
	assert.deepEqual(
		[...job.matchAll(/run_reference_sample ([123])/g)].map((match) => Number(match[1])),
		[1, 2, 3],
		"reference samples must run once per ordinal in order",
	);
	assert.deepEqual(
		[...job.matchAll(/run_candidate_sample ([123])/g)].map((match) => Number(match[1])),
		[1, 2, 3],
		"candidate samples must run once per ordinal in order",
	);
	assert.ok(
		job.indexOf("run_reference_sample 1") < job.indexOf("run_candidate_sample 1"),
		"reference samples must precede candidate samples",
	);
	assert.match(job, /run-unit-latency-sample\.mjs/);
	assert.match(job, /sample_failure=1/);
	const runner = readSampleRunner();
	for (const evidenceFile of ["stdout.log", "stderr.log", "status.txt"]) {
		assert.match(runner, new RegExp(`run-\\$\\{ordinal\\}\\.${evidenceFile.replace(".", "\\.")}`));
	}
	assert.match(runner, /cwd: workingDirectory/);
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
	assert.match(policy, /three-reference\/three-candidate/);
	assert.match(policy, /BACKEND_UNIT_LATENCY_RESULT: \$\{\{ needs\.backend-unit-latency\.result \}\}/);
});

test("the unit-latency job verifies the pinned reference before creating its worktree", () => {
	const job = unitLatencyJob(readWorkflow());

	assert.match(job, /reference_ready=0/);
	assert.match(job, /git fetch --no-tags origin "\$UNIT_REFERENCE_COMMIT"/);
	assert.match(job, /git cat-file -e "\$\{UNIT_REFERENCE_COMMIT\}\^\{commit\}"/);
	assert.match(job, /reference-fetch\.status\.txt/);
	assert.match(job, /reference commit fetch\/verification/);
	assert.match(job, /reference checkout: unavailable after pinned commit fetch\/verification/);
	const fetchIndex = job.indexOf("git fetch --no-tags origin");
	const worktreeIndex = job.indexOf("git worktree add --detach");
	assert.ok(fetchIndex >= 0 && fetchIndex < worktreeIndex, "reference fetch must precede worktree creation");
	assert.ok(
		job.indexOf('if [ "$reference_ready" -eq 1 ]; then', fetchIndex) < worktreeIndex,
		"worktree creation must be guarded by successful reference verification",
	);
});

test("the Make entrypoint keeps ordinary flags and exposes additive evidence controls", () => {
	const makefile = readMakefile();

	assert.match(makefile, /UNIT_TIMING_OUTPUT \?=/);
	assert.match(makefile, /UNIT_LATENCY_BUDGET \?= docs\/internal\/baselines\/go-unit-lane-latency-budget\.v2\.json/);
	assert.match(makefile, /UNIT_LATENCY_HISTORICAL_SAMPLES \?= docs\/internal\/development\/plans\/unit-test-optimization-c01-wire-timeout-witness\/baseline-make-run-1-replacement\.v2\.json/);
	assert.match(makefile, /UNIT_LATENCY_REFERENCE_SAMPLES \?= \.artifacts\/unit-latency\/reference\/run-1\.v2\.json,\.artifacts\/unit-latency\/reference\/run-2\.v2\.json,\.artifacts\/unit-latency\/reference\/run-3\.v2\.json/);
	assert.match(makefile, /UNIT_LATENCY_SAMPLES \?= \.artifacts\/unit-latency\/candidate\/run-1\.v2\.json,\.artifacts\/unit-latency\/candidate\/run-2\.v2\.json,\.artifacts\/unit-latency\/candidate\/run-3\.v2\.json/);
	assert.match(makefile, /unitlane -jobs \$\(UNIT_DEFAULT_JOBS\) -count=1 -timeout \$\(GO_TEST_TIMEOUT\)/);
	assert.match(makefile, /test-unit-latency-budget:/);
	assert.match(makefile, /unitlanebudget -mode final -budget/);
	assert.match(makefile, /-historical-samples/);
	assert.match(makefile, /-reference-samples/);
	assert.match(makefile, /-manifest/);
	assert.match(makefile, /unit-latency-workflow\.test\.mjs/);
});

test("the sample runner preserves ordered evidence across two checkout directories", () => {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "unit-latency-cohorts-"));
	try {
		const referenceCheckout = join(temporaryDirectory, "reference-checkout");
		const candidateCheckout = join(temporaryDirectory, "candidate-checkout");
		const referenceEvidence = join(temporaryDirectory, "evidence", "reference");
		const candidateEvidence = join(temporaryDirectory, "evidence", "candidate");
		const orderPath = join(temporaryDirectory, "execution-order.log");
		const fixturePath = join(temporaryDirectory, "fixture-sample.mjs");
		mkdirSync(referenceCheckout);
		mkdirSync(candidateCheckout);
		writeFileSync(
			fixturePath,
			[
				'import { appendFileSync, writeFileSync } from "node:fs";',
				'const [orderPath, label, timingPath] = process.argv.slice(2);',
				'appendFileSync(orderPath, `${label}:${process.cwd()}\\n`);',
				'process.stdout.write(`${label} stdout\\n`);',
				'process.stderr.write(`${label} stderr\\n`);',
				'writeFileSync(timingPath, JSON.stringify({ version: 2, complete: label !== "candidate-2" }) + "\\n");',
				'if (label === "candidate-2") process.exitCode = 17;',
			].join("\n"),
			"utf8",
		);

		const captures = [
			["reference", referenceCheckout, referenceEvidence],
			["candidate", candidateCheckout, candidateEvidence],
		];
		const statuses = [];
		for (const [cohort, checkout, outputDirectory] of captures) {
			for (const ordinal of [1, 2, 3]) {
				const label = `${cohort}-${ordinal}`;
				const timingPath = join(outputDirectory, `run-${ordinal}.v2.json`);
				const result = spawnSync(
					process.execPath,
					[sampleRunner, String(ordinal), outputDirectory, process.execPath, fixturePath, orderPath, label, timingPath],
					{
						cwd: repositoryRoot,
						env: { ...process.env, UNIT_LATENCY_SAMPLE_CWD: checkout },
						encoding: "utf8",
					},
				);
				statuses.push(result.status);
				for (const suffix of ["stdout.log", "stderr.log", "status.txt"]) {
					assert.equal(existsSync(join(outputDirectory, `run-${ordinal}.${suffix}`)), true, `${label} ${suffix} missing`);
				}
				assert.equal(existsSync(timingPath), true, `${label} timing JSON missing`);
				assert.equal(readFileSync(join(outputDirectory, `run-${ordinal}.stdout.log`), "utf8"), `${label} stdout\n`);
				assert.equal(readFileSync(join(outputDirectory, `run-${ordinal}.stderr.log`), "utf8"), `${label} stderr\n`);
			}
		}

		assert.deepEqual(statuses, [0, 0, 0, 0, 1, 0]);
		assert.deepEqual(
			readFileSync(orderPath, "utf8").trimEnd().split("\n"),
			[
				`reference-1:${referenceCheckout}`,
				`reference-2:${referenceCheckout}`,
				`reference-3:${referenceCheckout}`,
				`candidate-1:${candidateCheckout}`,
				`candidate-2:${candidateCheckout}`,
				`candidate-3:${candidateCheckout}`,
			],
		);
		assert.equal(readFileSync(join(candidateEvidence, "run-2.status.txt"), "utf8"), "exit_status=17\n");
	} finally {
		rmSync(temporaryDirectory, { recursive: true, force: true });
	}
});

test("the sample runner retains all evidence and fails on a failed command", () => {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "unit-latency-sample-"));
	try {
		const artifactDirectory = join(temporaryDirectory, "evidence");
		const timingPath = join(artifactDirectory, "run-1.v2.json");
		const fixturePath = join(temporaryDirectory, "failed-sample.mjs");
		writeFileSync(
			fixturePath,
			[
				'import { writeFileSync } from "node:fs";',
				'process.stdout.write("sample stdout\\n");',
				'process.stderr.write("sample stderr\\n");',
				'writeFileSync(process.argv[2], JSON.stringify({ version: 2, complete: false }) + "\\n");',
				"process.exitCode = 23;",
			].join("\n"),
			"utf8",
		);

		const result = spawnSync(
			process.execPath,
			[sampleRunner, "1", artifactDirectory, process.execPath, fixturePath, timingPath],
			{ cwd: repositoryRoot, encoding: "utf8" },
		);

		assert.equal(result.status, 1, `runner stderr: ${result.stderr}`);
		const evidencePaths = [
			join(artifactDirectory, "run-1.stdout.log"),
			join(artifactDirectory, "run-1.stderr.log"),
			timingPath,
			join(artifactDirectory, "run-1.status.txt"),
		];
		for (const evidencePath of evidencePaths) {
			assert.equal(existsSync(evidencePath), true, `missing evidence: ${evidencePath}`);
		}
		assert.equal(readFileSync(evidencePaths[0], "utf8"), "sample stdout\n");
		assert.equal(readFileSync(evidencePaths[1], "utf8"), "sample stderr\n");
		assert.deepEqual(JSON.parse(readFileSync(timingPath, "utf8")), { version: 2, complete: false });
		assert.equal(readFileSync(evidencePaths[3], "utf8"), "exit_status=23\n");
	} finally {
		rmSync(temporaryDirectory, { recursive: true, force: true });
	}
});
