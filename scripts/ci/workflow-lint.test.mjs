import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	discoverWorkflowFiles,
	runWorkflowLint,
	validateControlledRawFailureWorkflowContract,
	validateFunctionalDiagnosticsArtifactWorkflowContract,
} from "./workflow-lint.mjs";

test("workflow lint validates the guarded raw failure capture and evidence handoff", () => {
	const workflow = `name: CI
on:
  pull_request:
    types: [opened, synchronize, reopened, labeled, unlabeled]
jobs:
  backend-coverage:
    steps:
      - name: Select controlled raw failure fixture
        if: >-
          matrix.suite == 'functional' &&
          github.event_name == 'pull_request' &&
          github.event.action == 'labeled' &&
          github.event.pull_request.number == 2637 &&
          github.event.pull_request.head.ref == 'factory-reliability-functional-raw-evidence-20260923' &&
          github.event.label.name == 'ci-controlled-raw-failure'
        uses: actions/github-script@v7
        with:
          script: |
            const eventHead = context.payload.pull_request?.head?.sha;
            const { data: pullRequest } = await github.rest.pulls.get({ pull_number: 2637 });
            const liveHead = pullRequest.head.sha;
            if (!eventHead || eventHead !== liveHead) return;
            core.exportVariable("FUNCTIONAL_RAW_FAILURE_WITNESS", "1");
            core.exportVariable("FUNCTIONAL_RAW_FAILURE_RENDEZVOUS", process.env.RUNNER_TEMP + "/pr2637-raw-failure-" + liveHead);
            core.exportVariable("FUNCTIONAL_RAW_FAILURE_RENDEZVOUS_TIMEOUT", "90s");
            core.exportVariable("FUNCTIONAL_TEST_VIZ_PACKAGES", "github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailure github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailurepeer");
            core.exportVariable("FUNCTIONAL_QUARANTINE", "");
      - name: Run Linux functional coverage with concurrent quarantine verification
        run: bash scripts/ci/run-functional-coverage-with-quarantine.sh
      - name: Publish controlled raw failure interleaving witness
        if: always() && matrix.suite == 'functional'
        shell: bash
        run: |
          if [[ -z "\${FUNCTIONAL_RAW_FAILURE_RENDEZVOUS:-}" ]]; then exit 0; fi
          source="$FUNCTIONAL_RAW_FAILURE_RENDEZVOUS/interleaving.jsonl"
          target=".artifacts/functional-test-viz/raw-failure-interleaving.jsonl"
          test -s "$source" || exit 1
          cp "$source" "$target"
      - name: Upload functional test diagnostics
        if: always() && matrix.suite == 'functional'
        uses: actions/upload-artifact@v4
        with:
          name: functional-test-diagnostics
          path: |
            .artifacts/functional-test-viz/raw-failure-interleaving.jsonl
      - name: Finish
        run: true
`;

	assert.deepEqual(validateControlledRawFailureWorkflowContract({ workflow }), {
		name: "controlled-raw-failure-workflow",
		status: "pass",
	});
	assert.throws(
		() =>
			validateControlledRawFailureWorkflowContract({
				workflow: workflow.replace("github.event.label.name == 'ci-controlled-raw-failure'", "github.event.label.name == 'other'")
			}),
		/workflow contract failed: controlled raw failure selector must include github\.event\.label\.name/,
	);
	assert.throws(
		() =>
			validateControlledRawFailureWorkflowContract({
				workflow: workflow.replace("raw-failure-interleaving.jsonl\n", "other-evidence.jsonl\n"),
			}),
		/workflow contract failed: functional diagnostics artifact must include the raw failure interleaving witness/,
	);
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
		log(message) {
			messages.push(message);
		},
	});
	assert.equal(result.status, 0);
	assert.ok(result.workflowFiles.length > 0);
	assert.deepEqual(messages, [
		`WORKFLOW_LINT_FILE_COUNT=${result.workflowFiles.length}`,
		`WORKFLOW_LINT_OK files=${result.workflowFiles.length}`,
	]);
});
