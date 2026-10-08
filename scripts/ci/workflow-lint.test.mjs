import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { copyFile, mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	discoverWorkflowFiles,
	runWorkflowLint,
	validateCIJobGrowth,
	validateCIJobGrowthFromHistory,
	validateFunctionalDiagnosticsArtifactWorkflowContract,
	validateFrontendSharedSetupWorkflowContract,
	validateConsolidatedCIWorkflowContract,
} from "./workflow-lint.mjs";

test("CI job guard accepts legacy jobs, deletion and formatting, but rejects added IDs", () => {
	const baselineWorkflow = "name: CI\njobs:\n  unit:\n    steps:\n      - run: go test -race ./pkg/example\n  legacy:\n    steps: []\n";
	for (const workflow of [baselineWorkflow, baselineWorkflow.replace("  legacy:\n    steps: []\n", ""),
		baselineWorkflow.replace("  unit:", "  'unit': # formatting").replaceAll("\n", "\r\n")]) {
		assert.equal(validateCIJobGrowth({ workflow, baselineWorkflow }).status, "pass");
	}
	assert.throws(() => validateCIJobGrowth({ baselineWorkflow, workflow: `${baselineWorkflow}  witness:\n    steps: []\n` }),
		/added job IDs witness; put checks in their primary suite job or lint instead/);
	for (const workflow of ["jobs: { unit: {} }", "jobs:\n  <<: *jobs", "jobs:\n  unit: *job", "jobs:\n  unit:\n  unit:\n", "jobs:\njobs:\n"]) {
		assert.throws(() => validateCIJobGrowth({ workflow, baselineWorkflow }), /CI job guard/);
	}
});

test("CI job guard fails closed on absent merge base or unreadable baseline", () => {
	for (const failure of [{ status: 128 }, { error: new Error("git missing") }, { status: 0, stdout: "" }]) {
		assert.throws(() => validateCIJobGrowthFromHistory({ spawn: () => failure }), /comparison history unavailable.*fetch full origin\/main/);
	}
	let calls = 0;
	assert.throws(() => validateCIJobGrowthFromHistory({ spawn: () => ++calls === 1 ? { status: 0, stdout: "deadbeef" } : { status: 128 } }),
		/comparison history unavailable.*\(show\)/);
});

test("real workflow lint CLI compares isolated Git history and reports added jobs", async (t) => {
	const actionlint = process.env.ACTIONLINT_BIN || "actionlint";
	if (spawnSync(actionlint, ["-version"], { windowsHide: true }).error) {
		t.skip("pinned actionlint is supplied by hosted Workflow proof");
		return;
	}
	const root = await mkdtemp(join(tmpdir(), "workflow-lint-history-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	await mkdir(join(root, "scripts/ci"), { recursive: true });
	await mkdir(join(root, ".github/workflows"), { recursive: true });
	// Include the checker's transitive lint dependencies in the isolated CLI fixture.
	for (const file of [
		"scripts/ci/workflow-lint.mjs",
		"scripts/ci/verification-plans.mjs",
		"scripts/ci/backend-lint-workflow.mjs",
		"scripts/ci/backend-lint-report.mjs",
		"scripts/ci/backend-lint-policy.mjs",
		"scripts/ci/runner-parallelism.mjs",
		"Makefile",
	]) {
		await copyFile(file, join(root, file));
	}
	for (const file of discoverWorkflowFiles()) await copyFile(file, join(root, ".github/workflows", file.split(/[\\/]/).at(-1)));
	const git = (...args) => {
		const result = spawnSync("git", args, { cwd: root, encoding: "utf8", windowsHide: true });
		assert.equal(result.status, 0, result.stderr);
		return result.stdout.trim();
	};
	git("init", "--quiet");
	git("add", ".");
	git("-c", "user.name=Lint Fixture", "-c", "user.email=lint@example.invalid", "commit", "--quiet", "-m", "baseline");
	git("update-ref", "refs/remotes/origin/main", git("rev-parse", "HEAD"));
	const cli = () => spawnSync(process.execPath, ["scripts/ci/workflow-lint.mjs", "--actionlint", actionlint], {
		cwd: root, encoding: "utf8", windowsHide: true,
	});
	const workflowPath = join(root, ".github/workflows/ci.yml");
	const baseline = readFileSync(workflowPath, "utf8");
	await writeFile(workflowPath, `${baseline}\n# Formatting-only edit\n`);
	let result = cli();
	assert.equal(result.status, 0, result.stderr);
	assert.match(result.stdout, /WORKFLOW_LINT_STATIC_CONTRACTS_OK/);
	await writeFile(workflowPath, `${baseline}\n  added-witness:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo witness\n`);
	result = cli();
	assert.equal(result.status, 1);
	assert.match(result.stderr, /added job IDs added-witness/);
	await writeFile(workflowPath, baseline);
	git("update-ref", "-d", "refs/remotes/origin/main");
	result = cli();
	assert.equal(result.status, 1);
	assert.match(result.stderr, /comparison history unavailable/);
});

test("consolidated workflow checker identifies the invalid proof or result mapping", () => {
	const workflow = `name: fixture
jobs:
  classify:
    name: Verification Setup
    outputs:
      docs_result: \${{ steps.docs-reference.outcome }}
    steps:
      - run: go install example/actionlint@v1.7.12
      - run: bash scripts/ci/run-workflow-verification.sh
      - if: success() || failure()
        run: make docs-reference-smoke
  api-pr-verification:
    if: always() && github.event_name == 'pull_request' && needs.classify.outputs.run_api_package != 'false'
    steps:
      - run: bash scripts/ci/run-api-pr-verification.sh
  development-package:
    with:
      run_api_package: \${{ github.event_name != 'pull_request' && needs.classify.outputs.run_api_package != 'false' }}
  verification-policy:
    env:
      DOCS_RESULT: \${{ needs.classify.outputs.docs_result }}
      WORKFLOW_LINT_RESULT: \${{ needs.classify.result }}
      API_RESULT: \${{ github.event_name == 'pull_request' && needs.api-pr-verification.result || needs.development-package.outputs.api_package_result }}
      API_CANDIDATE_RESULT: \${{ github.event_name == 'pull_request' && needs.api-pr-verification.result || needs.development-package.outputs.api_candidate_result }}
      API_INDEPENDENT: \${{ github.event_name == 'pull_request' }}
`;
	for (const [input, diagnostic] of [
		["jobs:\n", "workflow contract is missing job: classify"],
		[workflow.replace("actionlint@v1.7.12", "actionlint@latest"), "workflow contract failed: retain shared setup proof and fail-closed selection"],
		[workflow.replace("run: make docs-reference-smoke", "run: true"), "workflow contract failed: retain shared setup proof and fail-closed selection"],
		[workflow.replace("run: bash scripts/ci/run-api-pr-verification.sh", "run: true"), "workflow contract failed: retain API proofs"],
		[workflow.replace("DOCS_RESULT:", "OTHER_RESULT:"), "workflow contract failed: map merged required proof"],
		[workflow.replace("API_INDEPENDENT:", "OTHER_FLAG:"), "workflow contract failed: API-only PR does not require reusable children"],
	]) {
		assert.throws(() => validateConsolidatedCIWorkflowContract({ workflow: input }), { message: diagnostic });
	}
});

test("frontend workflow checker explains invalid setup, proof and policy inputs", () => {
	const workflow = `name: fixture
env:
  BUN_VERSION: 1.3.12
jobs:
  frontend:
    needs: classify
    if: always() && github.event_name != 'push' && needs.classify.outputs.run_frontend != 'false'
    env:
      UI_COVERAGE_MAIN_MAX_WORKERS: "4"
    steps:
      - uses: oven-sh/setup-bun@v2
        with:
          bun-version: \${{ env.BUN_VERSION }}
      - uses: actions/cache@v4
        with:
          path: ~/.bun/install/cache
          key: frontend-bun-v1-\${{ runner.os }}-\${{ runner.arch }}-\${{ env.BUN_VERSION }}-\${{ hashFiles('ui/bun.lock') }}
      - run: cd ui && bun install --frozen-lockfile
      - run: make typecheck
      - name: Lint frontend
        if: success() || failure()
        run: make ui-lint
      - name: Run frontend unit and replay coverage
        if: success() || failure()
        run: make test-ui-coverage
      - name: Frontend component proof
        if: success() || failure()
        run: bash scripts/ci/run-frontend-verification.sh
        env:
          FRONTEND_SUITE: component
  frontend-browser:
    needs: classify
    if: always() && github.event_name != 'push' && needs.classify.outputs.run_frontend != 'false'
    steps:
      - uses: oven-sh/setup-bun@v2
        with:
          bun-version: \${{ env.BUN_VERSION }}
      - uses: actions/cache@v4
        with:
          path: ~/.bun/install/cache
          key: frontend-bun-v1-\${{ runner.os }}-\${{ runner.arch }}-\${{ env.BUN_VERSION }}-\${{ hashFiles('ui/bun.lock') }}
      - run: cd ui && bun install --frozen-lockfile
      - name: Restore Playwright Chromium
        uses: actions/cache@v4
        with:
          path: ~/.cache/ms-playwright
          key: playwright-chromium-v1-\${{ runner.os }}-\${{ runner.arch }}-\${{ hashFiles('ui/bun.lock') }}
      - run: make ui-install-playwright
      - name: Frontend proof
        run: bash scripts/ci/run-frontend-verification.sh
        env:
          FRONTEND_SUITE: browser
  verification-policy:
    needs: [frontend, frontend-browser]
    env:
      FRONTEND_RESULT: \${{ needs.frontend.result }}
      FRONTEND_COVERAGE_RESULT: \${{ needs.frontend.result }}
      FRONTEND_COMPONENT_RESULT: \${{ needs.frontend.result }}
      FRONTEND_BROWSER_RESULT: \${{ needs.frontend-browser.result }}
      FRONTEND_STORYBOOK_RESULT: \${{ needs.frontend-browser.result }}
`;
	assert.equal(validateFrontendSharedSetupWorkflowContract({ workflow }).status, "pass");
	for (const [before, after, diagnostic] of [
		["bun install --frozen-lockfile", "bun install", "install frozen dependencies"],
		["frontend-bun-v1-\${{ runner.os }}-\${{ runner.arch }}", "frontend-bun-v1", "cache identity must include platform, Bun and frozen lock"],
		["if: success() || failure()", "if: success()", "attempt later proof after failures, but not cancellation"],
		["run: make test-ui-coverage", "run: make ui-test", "retain the complete proof command"],
		["FRONTEND_COVERAGE_RESULT:", "OTHER_RESULT:", "static and coverage share the aggregate result"],
		["FRONTEND_SUITE: browser", "FRONTEND_SUITE: all", "complete suites use separate runners"],
		["FRONTEND_BROWSER_RESULT: \${{ needs.frontend-browser.result }}", "FRONTEND_BROWSER_RESULT: \${{ needs.frontend.result }}", "browser proofs share their own aggregate result"],
	]) {
		assert.throws(() => validateFrontendSharedSetupWorkflowContract({
			workflow: workflow.replace(before, after),
		}), { message: `workflow contract failed: ${diagnostic}` });
	}
});

test("functional diagnostics artifact uploads bounded raw failure evidence after the verdict", () => {
	const workflow = `name: CI
jobs:
  backend-coverage:
    steps:
      - name: Report functional coverage verdict
        if: always() && matrix.suite == 'functional'
        run: bash scripts/ci/publish-functional-coverage-verdict.sh
      - name: Upload functional test diagnostics
        if: always() && matrix.suite == 'functional'
        uses: actions/upload-artifact@v4
        with:
          name: functional-test-diagnostics
          path: |
            .artifacts/functional-test-viz/command.log
            .artifacts/functional-test-viz/raw-failures/index.json
            .artifacts/functional-test-viz/raw-failures/*.jsonl
          if-no-files-found: ignore
          retention-days: 14
      - name: Finish
        run: true
`;

	assert.deepEqual(validateFunctionalDiagnosticsArtifactWorkflowContract({ workflow }), {
		name: "functional-diagnostics-artifact-workflow",
		status: "pass",
	});
	assert.throws(
		() =>
			validateFunctionalDiagnosticsArtifactWorkflowContract({
				workflow: workflow.replace("raw-failures/index.json\n", ""),
			}),
		/workflow contract failed: functional diagnostics artifact must include the raw failure index/,
	);
});

test("the workflow lint runner passes every top-level YAML workflow to actionlint", async (t) => {
	const root = await mkdtemp(join(tmpdir(), "workflow-lint-files-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	await mkdir(join(root, "nested"));
	await writeFile(join(root, "z-workflow.yml"), "name: z\n");
	await writeFile(join(root, "a-workflow.yaml"), "name: a\n");
	await writeFile(join(root, "README.md"), "not a workflow\n");
	await writeFile(join(root, "nested", "ignored.yml"), "name: ignored\n");

	const calls = [];
	const messages = [];
	const result = runWorkflowLint({
		actionlint: "pinned-actionlint",
		workflowDirectory: root,
		spawn(command, args, options) {
			calls.push({ command, args, options });
			return { status: 0, signal: null };
		},
		log(message) {
			messages.push(message);
		},
	});

	const expectedFiles = discoverWorkflowFiles(root);
	assert.deepEqual(result.workflowFiles, expectedFiles);
	assert.deepEqual(calls, [
		{
			command: "pinned-actionlint",
			args: expectedFiles,
			options: { stdio: "inherit", windowsHide: true },
		},
	]);
	assert.deepEqual(messages, [`WORKFLOW_LINT_FILE_COUNT=2`, "WORKFLOW_LINT_OK files=2"]);
});

test("the required lint runner fails closed when no workflows or a linter error is observed", async (t) => {
	const emptyRoot = await mkdtemp(join(tmpdir(), "workflow-lint-empty-"));
	t.after(() => rm(emptyRoot, { recursive: true, force: true }));
	assert.throws(
		() => runWorkflowLint({ workflowDirectory: emptyRoot, spawn: () => ({ status: 0 }) }),
		/No GitHub Actions workflow files found/,
	);

	const workflowRoot = await mkdtemp(join(tmpdir(), "workflow-lint-failure-"));
	t.after(() => rm(workflowRoot, { recursive: true, force: true }));
	await writeFile(join(workflowRoot, "workflow.yml"), "name: workflow\n");
	assert.throws(
		() =>
			runWorkflowLint({
				workflowDirectory: workflowRoot,
				spawn: () => ({ status: 1, signal: null }),
			}),
		/Workflow schema lint failed with exit code 1/,
	);
});

test("actionlint rejects runner context in shell when the pinned binary is available", async (t) => {
	const version = spawnSync(process.env.ACTIONLINT_BIN || "actionlint", ["-version"], {
		encoding: "utf8",
		windowsHide: true,
	});
	if (version.error) {
		t.skip("actionlint is installed by the hosted workflow-lint job");
		return;
	}

	const root = await mkdtemp(join(tmpdir(), "workflow-lint-schema-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	const invalidWorkflow = join(root, "invalid.yml");
	await writeFile(
		invalidWorkflow,
		`name: invalid\n\non: workflow_dispatch\n\njobs:\n  build:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo invalid\n        shell: \${{ runner.os == 'Windows' && 'msys2 {0}' || 'bash' }}\n`,
	);
	const result = spawnSync(process.env.ACTIONLINT_BIN || "actionlint", [invalidWorkflow], {
		encoding: "utf8",
		windowsHide: true,
	});
	assert.notEqual(result.status, 0);
	assert.match(`${result.stdout}\n${result.stderr}`, /context "runner" is not allowed here/);
});

test("the checked-in workflow set passes the executable schema-lint gate", (t) => {
	const actionlint = process.env.ACTIONLINT_BIN || "actionlint";
	const version = spawnSync(actionlint, ["-version"], { encoding: "utf8", windowsHide: true });
	if (version.error) {
		t.skip("actionlint is installed by the hosted workflow-lint job");
		return;
	}

	const messages = [];
	const result = runWorkflowLint({
		actionlint,
		workflowDirectory: join(process.cwd(), ".github", "workflows"),
		validateRepositoryContracts: true,
		log(message) {
			messages.push(message);
		},
	});
	assert.equal(result.status, 0);
	assert.ok(result.workflowFiles.length > 0);
	assert.deepEqual(messages, [
		`WORKFLOW_LINT_FILE_COUNT=${result.workflowFiles.length}`,
		"WORKFLOW_LINT_STATIC_CONTRACTS_OK",
		`WORKFLOW_LINT_OK files=${result.workflowFiles.length}`,
	]);
});
