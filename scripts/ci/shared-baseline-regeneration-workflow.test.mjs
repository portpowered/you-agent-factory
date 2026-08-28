import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

import {
	SHARED_BASELINE_BOT_BRANCH,
	SHARED_BASELINE_PATHS,
	SHARED_BASELINE_PR_TITLE,
	parsePorcelainPaths,
	planReconciliation,
	renderPullRequestBody,
	selectSourceWorkflowRun,
	validateAllowlistedPaths,
} from "./shared-baseline-regeneration-workflow.mjs";

const SHA = (character) => character.repeat(40);
const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const workflow = readFileSync(
	join(repositoryRoot, ".github", "workflows", "regenerate-shared-ci-baselines.yml"),
	"utf8",
);
const ciWorkflow = readFileSync(join(repositoryRoot, ".github", "workflows", "ci.yml"), "utf8");
const makefile = readFileSync(join(repositoryRoot, "Makefile"), "utf8");
const helper = join(repositoryRoot, "scripts", "ci", "shared-baseline-regeneration-workflow.mjs");

test("enumerates exactly the classified snapshots and wires every merged writer", () => {
	assert.deepEqual(SHARED_BASELINE_PATHS, [
		"docs/internal/baselines/deadcode-baseline.txt",
		"docs/internal/baselines/go-unit-lane-latency-budget.v1.json",
		"docs/internal/baselines/ownership-inventory.json",
		"docs/internal/projects/packaged-service-structure/ownership-path-lease-freeze.json",
		"docs/internal/projects/packaged-service-structure/operator-settings-root-go-inventory.json",
		"docs/internal/projects/packaged-service-structure/operator-settings-top-level-inventory.json",
		"docs/internal/projects/packaged-service-structure/provider-sessions-root-go-inventory.json",
		"docs/internal/projects/packaged-service-structure/provider-sessions-top-level-inventory.json",
		"contracts/testdata/baseline/cli-commands.json",
		"contracts/testdata/baseline/cli-command-inputs.json",
		"contracts/testdata/baseline/mcp-tools.json",
	]);
	assert.equal(new Set(SHARED_BASELINE_PATHS).size, 11);

	const listed = spawnSync(process.execPath, [helper, "list-paths"], { encoding: "utf8" });
	assert.equal(listed.status, 0, listed.stderr);
	assert.deepEqual(listed.stdout.trimEnd().split(/\r?\n/), SHARED_BASELINE_PATHS);

	for (const command of [
		'cd "$(BASELINE_REGEN_ROOT)" && $(GO) run ./cmd/unitlanebudget',
		'cd "$(BASELINE_REGEN_ROOT)" && $(GO) run ./cmd/ownershipinventoryfreeze',
		'cd "$(BASELINE_REGEN_ROOT)" && $(BASELINE_REGEN_CLI_UPDATE_ENV) $(GO) test ./pkg/transports/cli/commandidentity -run "^TestWriteProductionInventoryBaseline$$" -count=1',
		'cd "$(BASELINE_REGEN_ROOT)" && $(BASELINE_REGEN_CLI_UPDATE_ENV) $(GO) test ./pkg/transports/cli/cliinputs -run "^TestWriteProductionInputsInventoryBaseline$$" -count=1',
		'cd "$(BASELINE_REGEN_ROOT)" && $(GO) run ./cmd/mcptoolinventorygen -root .',
	]) {
		assert.ok(makefile.includes(command), `Makefile is missing canonical writer command: ${command}`);
	}
});

test("the delivered workflow follows successful main CI and owns only the bot PR path", () => {
	assert.match(workflow, /workflow_run:\s+workflows: \[CI\]/);
	assert.match(workflow, /types: \[completed\]/);
	assert.match(workflow, /branches: \[main\]/);
	assert.match(workflow, /group: shared-ci-baseline-regeneration/);
	assert.match(workflow, /cancel-in-progress: true/);
	assert.match(workflow, /uses: actions\/create-github-app-token@v2/);
	assert.match(workflow, /app-id: \$\{\{ secrets\.BASELINE_BOT_APP_ID \}\}/);
	assert.match(workflow, /private-key: \$\{\{ secrets\.BASELINE_BOT_APP_PRIVATE_KEY \}\}/);
	assert.match(workflow, /echo \"GH_TOKEN=\$BOT_TOKEN\" >> \"\$GITHUB_ENV\"/);
	assert.match(workflow, /SHARED_BASELINE_PR_TITLE: [\"]?chore\(ci\): reconcile shared CI baselines[\"]?/);
	assert.match(workflow, /gh run download \"\$SOURCE_RUN_ID\"/);
	assert.match(workflow, /backend-unit-latency-evidence/);
	assert.match(workflow, /backend-deadcode-evidence/);
	assert.match(workflow, /DEADCODE_REPORT_PATH/);
	assert.match(workflow, /mapfile -t snapshot_paths < <\(node scripts\/ci\/shared-baseline-regeneration-workflow\.mjs list-paths\)/);
	assert.match(workflow, /git add -- "\$\{snapshot_paths\[@\]\}"/);
	assert.match(workflow, /BASELINE_REGEN_DEADCODE_REPORT=\"\$DEADCODE_REPORT_PATH\"/);
	assert.match(workflow, /SOURCE_EVENT: \$\{\{ github\.event\.workflow_run\.event \}\}/);
	assert.match(workflow, /SOURCE_REPOSITORY: \$\{\{ github\.event\.workflow_run\.head_repository\.full_name \}\}/);
	assert.match(workflow, /SOURCE_EVENT\" != \"push\"/);
	assert.match(workflow, /SOURCE_REPOSITORY\" != \"\$REPOSITORY\"/);
	assert.match(workflow, /make regenerate-shared-ci-baselines/);
	assert.match(workflow, /gh pr list/);
	assert.match(workflow, /gh pr create/);
	assert.match(workflow, /gh pr edit/);
	assert.match(workflow, /git push origin --delete \"\$BOT_BRANCH\"/);
	assert.match(workflow, /gh pr merge .*--auto .*--match-head-commit/);
	assert.match(workflow, /automation\/shared-ci-baselines/);
	assert.doesNotMatch(workflow, /for path in "\$DEADCODE_BASELINE"/);
	assert.doesNotMatch(workflow, /7438/);
});

test("Backend Lint publishes the normalized deadcode report consumed by reconciliation", () => {
	assert.match(ciWorkflow, /name: backend-deadcode-evidence/);
	assert.match(ciWorkflow, /path: bin\/deadcode-current\.txt/);
	assert.match(ciWorkflow, /if-no-files-found: error/);
});

test("selects completed CI runs for the default branch and lets artifacts judge drift", () => {
	assert.deepEqual(
		selectSourceWorkflowRun({
			workflowName: "CI",
			headBranch: "main",
			conclusion: "success",
		}),
		{ selected: true, reason: "completed successful CI run on the default branch" },
	);
	assert.equal(
		selectSourceWorkflowRun({
			workflowName: "CI",
			headBranch: "main",
			conclusion: "failure",
		}).selected,
		true,
	);
	for (const input of [
		{ workflowName: "Other", headBranch: "main", conclusion: "success" },
		{ workflowName: "CI", headBranch: "feature", conclusion: "success" },
		{ workflowName: "CI", headBranch: "main", conclusion: "cancelled" },
	]) {
		assert.equal(selectSourceWorkflowRun(input).selected, false);
	}
});

test("plans drift publication, no-diff cleanup, and exact-candidate reuse", () => {
	const common = { triggeringSha: SHA("a"), currentMainSha: SHA("a") };
	assert.equal(
		planReconciliation({ ...common, changedPaths: [SHARED_BASELINE_PATHS[0]] }).action,
		"publish",
	);
	assert.equal(planReconciliation({ ...common, changedPaths: [] }).action, "noop");
	assert.equal(
		planReconciliation({ ...common, changedPaths: [], existingPullRequest: true }).action,
		"close-existing",
	);
	assert.equal(
		planReconciliation({
			...common,
			changedPaths: [SHARED_BASELINE_PATHS[1]],
			candidateMatchesRemote: true,
			existingPullRequest: true,
		}).action,
		"reuse-pr",
	);
});

test("supersedes an overlapping run when main moved before publication", () => {
	const plan = planReconciliation({
		triggeringSha: SHA("a"),
		currentMainSha: SHA("b"),
		changedPaths: [SHARED_BASELINE_PATHS[0]],
	});
	assert.deepEqual(plan, {
		action: "superseded",
		publish: false,
		reason: `main moved from ${SHA("a")} to ${SHA("b")}; a newer run owns reconciliation`,
	});
});

test("fails before publication for generation errors and invalid revisions", () => {
	for (const plan of [
		planReconciliation({
			triggeringSha: SHA("a"),
			currentMainSha: SHA("a"),
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			generationError: "three samples are not comparable",
		}),
		planReconciliation({
			triggeringSha: "not-a-sha",
			currentMainSha: SHA("a"),
			changedPaths: [SHARED_BASELINE_PATHS[0]],
		}),
	]) {
		assert.equal(plan.action, "fail");
		assert.equal(plan.publish, false);
	}
});

test("parses status output and rejects every path outside the eleven-file allowlist", () => {
	const status = [
		...SHARED_BASELINE_PATHS.map((path) => ` M ${path}`),
		`R  old.txt -> ${SHARED_BASELINE_PATHS[0]}`,
	].join("\n");
	assert.deepEqual(
		parsePorcelainPaths(status),
		["old.txt", ...SHARED_BASELINE_PATHS].sort(),
	);
	assert.throws(
		() => validateAllowlistedPaths([...SHARED_BASELINE_PATHS, "docs/internal/baselines/other.txt"]),
		/unexpected path\(s\).*other\.txt/,
	);
	assert.throws(
		() => validateAllowlistedPaths([], { requireChanges: true }),
		/expected a generated change/,
	);
});

test("renders a bot PR body with the exact generated scope", () => {
	const body = renderPullRequestBody({
		sourceSha: SHA("a"),
		commitSha: SHA("b"),
		runUrl: "https://github.example/actions/runs/42",
		changedPaths: SHARED_BASELINE_PATHS,
	});
	assert.match(body, /shared-ci-baseline-regeneration/);
	assert.match(body, new RegExp(SHARED_BASELINE_BOT_BRANCH.replace("/", "\\/")));
	assert.ok(body.includes(SHARED_BASELINE_PR_TITLE));
	for (const path of SHARED_BASELINE_PATHS) assert.match(body, new RegExp(path.replaceAll("/", "\\/")));
});
