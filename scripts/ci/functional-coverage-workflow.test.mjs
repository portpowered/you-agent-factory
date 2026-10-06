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

test("functional coverage keeps module and dependency caches plus bounded compiler archives", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const unitSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'unit'", "      - uses: actions/setup-go@v5");
	const functionalSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'functional'", "      - name: Select functional runner parallelism");
	const functionalParallelism = stepSection(job, "      - name: Select functional runner parallelism", "      - name: Restore Go module cache");
	const moduleCache = stepSection(job, "      - name: Restore Go module cache", "      - name: Prefetch complete Go dependency graph");
	const modulePrefetch = stepSection(job, "      - name: Prefetch complete Go dependency graph", "      - name: Restore unit coverage Go build and test cache");

	assert.match(unitSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(unitSetup, /cache: false/);
	assert.doesNotMatch(unitSetup, /functional-coverage-|actions\/cache@v4/);

	assert.match(functionalSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(functionalSetup, /cache: false/);
	assert.match(functionalParallelism, /functional_jobs=8/);
	assert.match(functionalParallelism, /jobs=\$functional_jobs/);

	assert.match(moduleCache, /id: go-module-cache/);
	assert.match(moduleCache, /uses: actions\/cache@v4/);
	assert.match(moduleCache, /path: ~\/go\/pkg\/mod/);
	assert.doesNotMatch(moduleCache, /~\/\.cache\/go-build/);
	assert.match(
		moduleCache,
		/key: go-modules-v2-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/,
	);
	assert.match(moduleCache, /go-modules-v2-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-/);
	assert.match(moduleCache, /go-modules-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-/);
	assert.match(modulePrefetch, /run: go list -deps -test -mod=readonly \.\/\.\.\. > \/dev\/null/);
	assert.ok(
		job.indexOf("      - name: Restore Go module cache") <
			job.indexOf("      - name: Prefetch complete Go dependency graph"),
		"the complete module graph must be prefetched after cache restore",
	);

	assert.equal((job.match(/uses: actions\/cache@v4/g) ?? []).length, 1);
	assert.equal((job.match(/uses: actions\/cache\/restore@v4/g) ?? []).length, 3);
	assert.equal((job.match(/uses: actions\/cache\/save@v4/g) ?? []).length, 3);
	assert.doesNotMatch(job, /functional-coverage-build-|functional-go-build-cache/);
	assert.doesNotMatch(job, /functional-coverage-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/);
});

test("functional coverage build cache is deps-only, exact-key, and saved only from main", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const restore = stepSection(job, "      - name: Restore functional dependency build cache", "      - name: Build functional dependency build cache");
	const build = stepSection(job, "      - name: Build functional dependency build cache", "      - name: Save functional dependency build cache");
	const save = stepSection(job, "      - name: Save functional dependency build cache", "      - name: Apply bounded functional compiler archives");

	assert.match(restore, /uses: actions\/cache\/restore@v4/);
	assert.match(restore, /id: functional-deps-cache/);
	assert.match(restore, /steps\.functional-compiler-cache\.outputs\.cache-matched-key == ''/);
	assert.ok(job.indexOf("      - name: Restore bounded functional compiler archives") < job.indexOf("      - name: Restore functional dependency build cache"));
	assert.match(restore, /path: ~\/\.cache\/go-build/);
	assert.match(restore, /key: functional-deps-build-v1-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.mod', 'go\.sum'\) \}\}/);
	assert.doesNotMatch(restore, /restore-keys/);
	assert.doesNotMatch(restore, /matrix\.suite == 'unit'/);

	// Fill and save happen only on a main push miss, before any other step can
	// add module-package objects to the cache directory.
	for (const section of [build, save]) {
		assert.match(section, /matrix\.suite == 'functional' && github\.event_name == 'push' && github\.ref == 'refs\/heads\/main' && steps\.functional-deps-cache\.outputs\.cache-hit != 'true'/);
	}
	assert.match(build, /grep -v "\^\$\{module_path\}"/);
	assert.match(build, /runtime\/coverage/);
	assert.match(save, /uses: actions\/cache\/save@v4/);
	assert.match(save, /key: \$\{\{ steps\.functional-deps-cache\.outputs\.cache-primary-key \}\}/);
	assert.ok(
		job.indexOf("      - name: Save functional dependency build cache") <
			job.indexOf("      - name: Run Linux functional coverage with concurrent quarantine verification"),
		"the deps cache must be saved before module packages are compiled",
	);
});

test("functional compiler archives stay bounded and restore separately from the full Go cache", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	assert.doesNotMatch(job, /Restore functional coverage Go build cache/);
	assert.doesNotMatch(job, /Save functional coverage Go build cache/);
	const restore = stepSection(job, "      - name: Restore bounded functional compiler archives", "      - name: Restore functional dependency build cache");
	const capture = stepSection(job, "      - name: Capture bounded functional compiler archives", "      - name: Save bounded functional compiler archives");
	assert.match(restore, /path: \.artifacts\/functional-compiler-cache\/cache/);
	assert.doesNotMatch(restore, /path: ~\/\.cache\/go-build/);
	assert.match(restore, /functional-compiler-archives-v2/);
	assert.match(restore, /functional-compiler-archives-v1/);
	assert.match(capture, /steps\.functional-compiler-cache\.outputs\.cache-hit != 'true'/);
	assert.match(capture, /--max-bytes 2147483648/);
	assert.match(capture, /--job-start "\$JOB_START"/);
	const apply = stepSection(job, "      - name: Apply bounded functional compiler archives", "      - uses: actions/setup-node@v4");
	assert.match(apply, /if \[\[ -z "\$COMPILER_CACHE_MATCHED_KEY" \]\]/);
	assert.match(apply, /restore "\$compiler_cache" "\$\(go env GOCACHE\)"/);
	assert.match(apply, /echo "GOCACHE=\$compiler_cache" >> "\$GITHUB_ENV"/);
	assert.match(job, /FUNCTIONAL_COVERAGE_ACTION_CACHE_PRIMARY_KEY:/);
	assert.match(job, /FUNCTIONAL_COVERAGE_ACTION_CACHE_MATCHED_KEY:/);
	assert.match(job, /FUNCTIONAL_COVERAGE_ACTION_CACHE_EXACT_HIT: \$\{\{ steps\.functional-compiler-cache\.outputs\.cache-hit \|\| 'false' \}\}/);
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

test("pinned real ACP evidence belongs to backend integration, not functional coverage", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const coverageJob = jobSection(workflow, "backend-coverage");
	const integrationJob = jobSection(workflow, "backend-integration");

	assert.doesNotMatch(coverageJob, /INFINITE_YOU_RUN_ACPX_REAL_CLIENT|real-acpx-evidence/);
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

test("functional coverage retries failing tests once and publishes the flake ledger", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const supervisor = stepSection(job, "      - name: Run Linux functional coverage with concurrent quarantine verification", "      - name: Upload Linux quarantine package evidence");
	const record = stepSection(job, "      - name: Record functional flake ledger", "      - name: Upload functional flake ledger");
	const upload = stepSection(job, "      - name: Upload functional flake ledger", "      - name: Upload functional test diagnostics");

	assert.match(supervisor, /FUNCTIONAL_FLAKE_RETRY_MAX: "5"/);
	assert.match(supervisor, /FUNCTIONAL_FLAKE_LEDGER: \.artifacts\/functional-test-viz\/flake-ledger\.json/);
	assert.match(supervisor, /FUNCTIONAL_FLAKE_HEAD_SHA: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(record, /if: always\(\) && matrix\.suite == 'functional'/);
	assert.match(record, /hashFiles\('scripts\/ci\/flake-ledger-summary\.mjs'\) != ''/);
	assert.match(record, /node scripts\/ci\/flake-ledger-summary\.mjs --ledger \.artifacts\/functional-test-viz\/flake-ledger\.json/);
	assert.match(upload, /name: functional-flake-ledger/);
	assert.match(upload, /if-no-files-found: ignore/);
	assert.ok(
		job.indexOf("      - name: Report functional coverage verdict") < job.indexOf("      - name: Record functional flake ledger"),
		"the flake ledger is reported after the coverage verdict",
	);
});
