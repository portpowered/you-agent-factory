import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";

const workflowPath = join(process.cwd(), ".github", "workflows", "ci.yml");
const timingsPath = join(process.cwd(), "scripts", "ci", "functional-shard-timings.json");

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

function loadWorkflow() {
	return readFileSync(workflowPath, "utf8");
}

test("functional coverage runs as parallel shards whose count matches the workflow constant", () => {
	const workflow = loadWorkflow();
	const shard = jobSection(workflow, "backend-functional-shard");
	const declared = workflow.match(/^  FUNCTIONAL_SHARD_COUNT: "(\d+)"$/m);
	assert.ok(declared, "workflow env must declare FUNCTIONAL_SHARD_COUNT");
	const matrix = shard.match(/matrix:\n\s+shard: \[([0-9, ]+)\]/);
	assert.ok(matrix, "the shard job must declare a numeric shard matrix");
	const indexes = matrix[1].split(",").map((value) => Number(value.trim()));

	assert.equal(indexes.length, Number(declared[1]), "matrix length must equal FUNCTIONAL_SHARD_COUNT");
	assert.deepEqual(indexes, indexes.map((_, index) => index), "shard indexes must be exactly 0..N-1");
	assert.match(shard, /name: Backend Functional Coverage Shard \$\{\{ matrix\.shard \}\}/);
	assert.match(shard, /fail-fast: false/);
	assert.match(shard, /FUNCTIONAL_SHARD_INDEX: \$\{\{ matrix\.shard \}\}/);
	assert.match(shard, /FUNCTIONAL_SHARD_COUNT: \$\{\{ env\.FUNCTIONAL_SHARD_COUNT \}\}/);
	assert.match(shard, /run: \|\n\s+mkdir -p "\$FUNCTIONAL_SHARD_DIR"\n\s+make functional-coverage-shard 2>&1 \| tee/);
	assert.match(shard, /shell: bash/, "the shard step must run under bash -o pipefail so a failing make fails the step despite tee");
});

test("the shard timing table is checked in, versioned, and covers the shard matrix", () => {
	const timings = JSON.parse(readFileSync(timingsPath, "utf8"));
	assert.equal(timings.version, 1);
	assert.ok(Object.keys(timings.packages).length >= 100, "the table must cover the functional packages");
	for (const [name, seconds] of Object.entries(timings.packages)) {
		assert.ok(Number.isFinite(seconds) && seconds >= 0, `invalid timing for ${name}`);
	}
	assert.match(readFileSync(join(process.cwd(), "Makefile"), "utf8"), /FUNCTIONAL_SHARD_TIMINGS \?= scripts\/ci\/functional-shard-timings\.json/);
});

test("functional shards keep the module cache and a deps-only build cache saved only from one main shard", () => {
	const workflow = loadWorkflow();
	const job = jobSection(workflow, "backend-functional-shard");
	const moduleCache = stepSection(job, "      - name: Restore Go module cache", "      - name: Prefetch complete Go dependency graph");
	const modulePrefetch = stepSection(job, "      - name: Prefetch complete Go dependency graph", "      - name: Select functional runner parallelism");
	const parallelism = stepSection(job, "      - name: Select functional runner parallelism", "      # Deps-only functional build cache");
	const restore = stepSection(job, "      - name: Restore functional dependency build cache", "      - name: Build functional dependency build cache");
	const build = stepSection(job, "      - name: Build functional dependency build cache", "      - name: Save functional dependency build cache");
	const save = stepSection(job, "      - name: Save functional dependency build cache", "      - uses: actions/setup-node@v4");

	assert.match(moduleCache, /uses: actions\/cache@v4/);
	assert.match(moduleCache, /path: ~\/go\/pkg\/mod/);
	assert.doesNotMatch(moduleCache, /~\/\.cache\/go-build/);
	assert.match(
		moduleCache,
		/key: go-modules-v2-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/,
	);
	assert.match(modulePrefetch, /run: go list -deps -test -mod=readonly \.\/\.\.\. > \/dev\/null/);
	assert.match(parallelism, /functional_jobs=12/);
	assert.match(parallelism, /jobs=\$functional_jobs/);

	assert.match(restore, /uses: actions\/cache\/restore@v4/);
	assert.match(restore, /id: functional-deps-cache/);
	assert.match(restore, /path: ~\/\.cache\/go-build/);
	assert.match(restore, /key: functional-deps-build-v1-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.mod', 'go\.sum'\) \}\}/);
	assert.doesNotMatch(restore, /restore-keys/);

	// Fill and save happen only on a main push miss from shard 0, before any
	// other step can add module-package objects to the cache directory.
	for (const section of [build, save]) {
		assert.match(section, /matrix\.shard == 0 && github\.event_name == 'push' && github\.ref == 'refs\/heads\/main' && steps\.functional-deps-cache\.outputs\.cache-hit != 'true'/);
	}
	assert.match(build, /grep -v "\^\$\{module_path\}"/);
	assert.match(build, /runtime\/coverage/);
	assert.match(save, /uses: actions\/cache\/save@v4/);
	assert.match(save, /key: \$\{\{ steps\.functional-deps-cache\.outputs\.cache-primary-key \}\}/);
	assert.ok(
		job.indexOf("      - name: Save functional dependency build cache") <
			job.indexOf("      - name: Run functional coverage shard"),
		"the deps cache must be saved before module packages are compiled",
	);
	assert.doesNotMatch(job, /functional-coverage-build-|functional-go-build-cache|FUNCTIONAL_COVERAGE_ACTION_CACHE_/);
});

test("every shard uploads the files the aggregate requires, even after a failure", () => {
	const job = jobSection(loadWorkflow(), "backend-functional-shard");
	const upload = stepSection(job, "      - name: Upload functional coverage shard", "      - name: Upload functional raw failure evidence");

	assert.match(upload, /if: always\(\)/);
	assert.match(upload, /name: functional-coverage-shard-\$\{\{ matrix\.shard \}\}/);
	assert.match(upload, /if-no-files-found: error/);
	for (const file of ["shard-manifest.json", "coverage.out", "timing.json"]) {
		assert.match(upload, new RegExp(`functional-coverage-shard-\\$\\{\\{ matrix\\.shard \\}\\}/${file.replace(".", "\\.")}`));
	}
	const run = stepSection(job, "      - name: Run functional coverage shard", "      - name: Record functional flake ledger");
	assert.match(run, /FUNCTIONAL_FLAKE_RETRY_MAX: "5"/);
	assert.match(run, /FUNCTIONAL_FLAKE_LEDGER: \.artifacts\/functional-shards\/functional-coverage-shard-\$\{\{ matrix\.shard \}\}\/flake-ledger\.json/);
	assert.match(run, /FUNCTIONAL_SHORT: "false"/);
	assert.match(run, /timeout-minutes: 40/);
	const record = stepSection(job, "      - name: Record functional flake ledger", "      # The aggregate requires every file below");
	assert.match(record, /if: always\(\) && hashFiles\('scripts\/ci\/flake-ledger-summary\.mjs'\) != ''/);
	assert.match(record, /node scripts\/ci\/flake-ledger-summary\.mjs --ledger \.artifacts\/functional-shards\/functional-coverage-shard-\$\{\{ matrix\.shard \}\}\/flake-ledger\.json/);
});

test("the quarantine selector check, ratchet, and evidence run once in their own job", () => {
	const workflow = loadWorkflow();
	const quarantine = jobSection(workflow, "backend-functional-quarantine");
	const shard = jobSection(workflow, "backend-functional-shard");
	const aggregate = jobSection(workflow, "backend-functional-coverage");

	assert.match(quarantine, /name: Backend Functional Quarantine/);
	assert.match(quarantine, /run: bash scripts\/ci\/verify-functional-quarantine-packages\.sh/);
	assert.match(quarantine, /FUNCTIONAL_QUARANTINE_RATCHET: "true"/);
	assert.match(quarantine, /FUNCTIONAL_QUARANTINE_EVIDENCE_OUTPUT: \.artifacts\/functional-test-viz\/quarantine-package-evidence\.txt/);
	const upload = stepSection(quarantine, "      - name: Upload Linux quarantine package evidence", "\n  backend-");
	assert.match(upload, /if: always\(\)/);
	assert.match(upload, /if-no-files-found: error/);
	assert.doesNotMatch(shard, /verify-functional-quarantine-packages|run-functional-coverage-with-quarantine/);
	assert.doesNotMatch(aggregate, /verify-functional-quarantine-packages|run-functional-coverage-with-quarantine/);
	assert.match(readFileSync(join(process.cwd(), "scripts", "ci", "verify-functional-quarantine-packages.sh"), "utf8"), /-functional-quarantine-ratchet/);
});

test("the aggregate keeps the required check name and fails closed on any shard outcome", () => {
	const workflow = loadWorkflow();
	const job = jobSection(workflow, "backend-functional-coverage");
	const require = stepSection(job, "      - name: Require every functional shard and the quarantine check", "      - uses: actions/checkout@v4");

	assert.match(job, /\n    name: Backend Functional Coverage\n/);
	assert.match(job, /needs: \[classify, backend-functional-shard, backend-functional-quarantine\]/);
	assert.match(job, /if: always\(\) && needs\.classify\.outputs\.run_backend != 'false'/);
	assert.match(require, /SHARD_RESULT: \$\{\{ needs\.backend-functional-shard\.result \}\}/);
	assert.match(require, /QUARANTINE_RESULT: \$\{\{ needs\.backend-functional-quarantine\.result \}\}/);
	assert.match(require, /!= "success"/);
	assert.match(require, /exit "\$status"/);
	assert.ok(
		job.indexOf("      - name: Require every functional shard and the quarantine check") <
			job.indexOf("      - name: Merge functional coverage shards and run the coverage gate"),
		"the gate must not run before every shard result is required",
	);
	// Shard artifacts are only downloaded after the result gate, one directory per shard.
	const download = stepSection(job, "      - name: Download functional coverage shards", "      - name: Merge functional coverage shards");
	assert.match(download, /uses: actions\/download-artifact@v4/);
	assert.match(download, /pattern: functional-coverage-shard-\*/);
	assert.doesNotMatch(download, /merge-multiple/);
	assert.match(download, /path: \.artifacts\/functional-shards/);
});

test("the aggregate runs the unchanged gate over merged shards and publishes both status paths", () => {
	const workflow = loadWorkflow();
	const job = jobSection(workflow, "backend-functional-coverage");
	const gate = stepSection(job, "      - name: Merge functional coverage shards and run the coverage gate", "      # Ordinary test and coverage-gate failures");
	const verdict = stepSection(job, "      - name: Report functional coverage verdict", "      - name: Upload functional test diagnostics");

	assert.match(gate, /run: make functional-test-viz/);
	assert.match(gate, /FUNCTIONAL_MERGE_SHARDS: \.artifacts\/functional-shards/);
	assert.match(gate, /FUNCTIONAL_SHARD_COUNT: \$\{\{ env\.FUNCTIONAL_SHARD_COUNT \}\}/);
	assert.match(gate, /FUNCTIONAL_SHORT: "false"/);
	assert.match(gate, /FUNCTIONAL_GOCOVERAGE_EXIT_FILE: \.artifacts\/functional-test-viz\/gocoveragecheck-exit-code\.txt/);
	assert.doesNotMatch(gate, /FUNCTIONAL_COVERAGE_BUILD_DIAGNOSTICS|GO_COVERAGE_FLOOR_POLICY/, "the gate must not override floor policy");
	assert.match(verdict, /if: always\(\)/);
	assert.match(verdict, /run: bash scripts\/ci\/publish-functional-coverage-verdict\.sh/);
	assert.ok(
		job.indexOf("      - name: Merge functional coverage shards") < job.indexOf("      - name: Report functional coverage verdict"),
		"the verdict is reported after the gate",
	);
});

test("Verification Policy depends on the aggregate and folds its result into the backend result", () => {
	const workflow = loadWorkflow();
	const policy = jobSection(workflow, "verification-policy");

	assert.match(policy, /needs: \[[^\]]*\bbackend-coverage\b[^\]]*\bbackend-functional-coverage\b[^\]]*\]/);
	const backendResult = policy.match(/BACKEND_RESULT: (.*)\n/);
	assert.ok(backendResult, "Verification Policy must pass BACKEND_RESULT");
	assert.match(backendResult[1], /needs\.backend-functional-coverage\.result/);
	assert.match(backendResult[1], /needs\.backend-coverage\.result/);
});

test("pinned real ACP evidence belongs to backend integration, not functional coverage", () => {
	const workflow = loadWorkflow();
	const integrationJob = jobSection(workflow, "backend-integration");
	for (const name of ["backend-functional-shard", "backend-functional-coverage", "backend-coverage"]) {
		assert.doesNotMatch(jobSection(workflow, name), /INFINITE_YOU_RUN_ACPX_REAL_CLIENT|real-acpx-evidence/);
	}

	assert.match(integrationJob, /uses: actions\/setup-node@v4/);
	assert.match(integrationJob, /name: Build shared CLI artifact for compiled integration evidence/);
	assert.match(integrationJob, /go build -o \.artifacts\/integration\/bin\/you \.\/cmd\/factory/);
	assert.match(integrationJob, /name: Run backend integration tests with shared CLI artifact/);
	assert.match(integrationJob, /run: make test-integration/);
	assert.match(integrationJob, /INFINITE_YOU_PREBUILT_ARTIFACT: \$\{\{ github\.workspace \}\}\/\.artifacts\/integration\/bin\/you/);
	assert.match(integrationJob, /INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT: "1"/);
	assert.match(integrationJob, /name: Run required runtime metrics session probe/);
	assert.match(integrationJob, /TestProbeMetricsSessionUsesInstalledArtifact/);
	assert.match(integrationJob, /name: Upload runtime metrics session probe evidence/);
	assert.match(integrationJob, /runtime-metrics-session-probe\.log/);
	assert.ok(
		integrationJob.indexOf("name: Build shared CLI artifact for compiled integration evidence") <
			integrationJob.indexOf("name: Run backend integration tests with shared CLI artifact"),
		"the compiled artifact must be built before backend integration tests",
	);
	assert.ok(
		integrationJob.indexOf("name: Run backend integration tests with shared CLI artifact") <
			integrationJob.indexOf("name: Run required runtime metrics session probe"),
		"the required session probe must run after the shared artifact is supplied to the integration lane",
	);
	assert.match(integrationJob, /name: Run pinned real ACP integration/);
	assert.match(integrationJob, /INFINITE_YOU_RUN_ACPX_REAL_CLIENT: "1"/);
	assert.match(integrationJob, /INFINITE_YOU_REQUIRE_ACPX_REAL_CLIENT: "1"/);
	assert.match(integrationJob, /INFINITE_YOU_INTEGRATION_BINARY:/);
	assert.match(integrationJob, /\.artifacts\/integration\/real-acpx-evidence\.json/);
	assert.match(integrationJob, /name: Require pinned real ACP integration evidence/);
	assert.match(integrationJob, /if-no-files-found: error/);
});
