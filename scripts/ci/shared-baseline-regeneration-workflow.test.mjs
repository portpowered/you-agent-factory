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
	reconcileBotCandidate,
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
const helperSource = readFileSync(helper, "utf8");
const REPOSITORY = "portpowered/you-agent-factory";
const MAIN_SHA = SHA("a");
const BOT_BRANCH_SHA = SHA("b");
const GENERATED_SHA = SHA("c");
const SOURCE_RUN_URL = "https://github.example/actions/runs/42";

function createControlledCommandEdge({
	currentMainSha = MAIN_SHA,
	remotePaths = [],
	matchingPaths = SHARED_BASELINE_PATHS,
	stagedPaths = remotePaths,
	pullRequestLists = [[]],
	metadata = {},
	generatedSha = GENERATED_SHA,
	branchLookupStatus = 0,
	failWhen,
} = {}) {
	const calls = [];
	const equalPaths = new Set(matchingPaths);
	const pullRequestMetadata = {
		baseRefName: "main",
		headRefName: SHARED_BASELINE_BOT_BRANCH,
		headRefOid: generatedSha,
		isDraft: false,
		files: remotePaths.map((path) => ({ path })),
		...metadata,
	};
	if (Array.isArray(pullRequestMetadata.files)) {
		pullRequestMetadata.files = pullRequestMetadata.files.map((file) =>
			typeof file === "string" ? { path: file } : file,
		);
	}
	let mainRevisionReads = 0;
	let pullRequestListReads = 0;

	function resolveCurrentMain() {
		if (!Array.isArray(currentMainSha)) return currentMainSha;
		const index = Math.min(mainRevisionReads, currentMainSha.length - 1);
		mainRevisionReads += 1;
		return currentMainSha[index];
	}

	function run(command, args, options = {}) {
		const call = { command, args: [...args], options: { ...options } };
		calls.push(call);
		const failure = failWhen?.(call);
		if (failure) {
			if (failure instanceof Error) throw failure;
			return { status: typeof failure === "number" ? failure : 1, stderr: "simulated command failure" };
		}

		if (command === "git") {
			if (args[0] === "rev-parse" && args[1] === "origin/main") {
				return { status: 0, stdout: `${resolveCurrentMain()}\n` };
			}
			if (args[0] === "rev-parse" && args[1] === "HEAD") {
				return { status: 0, stdout: `${generatedSha}\n` };
			}
			if (args[0] === "diff" && args[1] === "--name-only") {
				const paths = args[3]?.startsWith("origin/") ? remotePaths : stagedPaths;
				return { status: 0, stdout: paths.join("\n") };
			}
			if (args[0] === "diff" && args[1] === "--cached" && args[2] === "--name-only") {
				const paths = stagedPaths;
				return { status: 0, stdout: paths.join("\n") };
			}
			if (args[0] === "diff" && args[1] === "--quiet") {
				const path = args[args.indexOf("--") + 1];
				return { status: equalPaths.has(path) ? 0 : 1, stdout: "" };
			}
			if (args[0] === "ls-remote") {
				return { status: branchLookupStatus, stdout: branchLookupStatus === 0 ? "ref\n" : "" };
			}
			return { status: 0, stdout: "" };
		}

		if (command === "gh" && args[0] === "pr" && args[1] === "list") {
			const index = Math.min(pullRequestListReads, pullRequestLists.length - 1);
			pullRequestListReads += 1;
			return { status: 0, stdout: JSON.stringify(pullRequestLists[index]) };
		}
		if (command === "gh" && args[0] === "pr" && args[1] === "create") {
			return { status: 0, stdout: "https://github.example/pull/42\n" };
		}
		if (command === "gh" && args[0] === "pr" && args[1] === "view" && args.includes("--jq")) {
			return { status: 0, stdout: "42\n" };
		}
		if (command === "gh" && args[0] === "pr" && args[1] === "view") {
			return { status: 0, stdout: JSON.stringify(pullRequestMetadata) };
		}
		return { status: 0, stdout: "" };
	}

	return { calls, run };
}

function commandText(call) {
	return `${call.command} ${call.args.join(" ")}`;
}

function callsMatching(calls, predicate) {
	return calls.filter(predicate).map(commandText);
}

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
	assert.match(workflow, /BASELINE_REGEN_DEADCODE_REPORT=\"\$DEADCODE_REPORT_PATH\"/);
	assert.match(workflow, /SOURCE_EVENT: \$\{\{ github\.event\.workflow_run\.event \}\}/);
	assert.match(workflow, /SOURCE_REPOSITORY: \$\{\{ github\.event\.workflow_run\.head_repository\.full_name \}\}/);
	assert.match(workflow, /SOURCE_EVENT\" != \"push\"/);
	assert.match(workflow, /SOURCE_REPOSITORY\" != \"\$REPOSITORY\"/);
	assert.match(workflow, /if \[\[ -z "\$BOT_TOKEN" \]\]/);
	assert.match(workflow, /refusing to treat the branch as absent/);
	assert.match(workflow, /make regenerate-shared-ci-baselines/);
	assert.match(workflow, /node scripts\/ci\/shared-baseline-regeneration-workflow\.mjs reconcile/);
	assert.match(workflow, /CHANGED_PATHS: \$\{\{ steps\.candidate\.outputs\.paths \}\}/);
	assert.match(workflow, /BOT_BRANCH_SHA: \$\{\{ steps\.prepare\.outputs\.bot_branch_sha \}\}/);
	assert.match(helperSource, /\"pr\",\s+\"list\"/);
	assert.match(helperSource, /\"pr\",\s+\"create\"/);
	assert.match(helperSource, /\"pr\",\s+\"edit\"/);
	assert.match(helperSource, /\"push\",\s+\"origin\",\s+\"--delete\"/);
	assert.match(helperSource, /\"pr\",\s+\"merge\"/);
	assert.match(helperSource, /\"--match-head-commit\"/);
	assert.match(helperSource, /allowFailure/);
	assert.match(workflow, /automation\/shared-ci-baselines/);
	assert.doesNotMatch(workflow, /for path in "\$DEADCODE_BASELINE"/);
	assert.doesNotMatch(workflow, /7438/);
	assert.ok(
		workflow.indexOf("name: Export the bot credential for later steps") <
			workflow.indexOf("name: Check out the delivered main revision"),
		"the bot credential must be validated before checkout or mutation steps",
	);
});

test("F-18/F-19 keep overlapping runs serialized and restart cancelled work from source main", () => {
	assert.match(workflow, /group: shared-ci-baseline-regeneration/);
	assert.match(workflow, /cancel-in-progress: true/);
	assert.match(workflow, /set -euo pipefail/);
	assert.match(workflow, /git switch --detach "\$SOURCE_SHA"/);
	assert.match(workflow, /git switch -C "\$BOT_BRANCH" "\$SOURCE_SHA"/);
	assert.doesNotMatch(workflow, /continue-on-error: true/);
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

test("F-01 controlled edge closes an obsolete PR and deletes its branch without committing", () => {
	const edge = createControlledCommandEdge({
		pullRequestLists: [[{ number: 19, url: "https://github.example/pull/19" }]],
		branchLookupStatus: 0,
	});
	const result = reconcileBotCandidate({
		 repository: REPOSITORY,
		mainSha: MAIN_SHA,
		botBranchExists: true,
		botBranchSha: BOT_BRANCH_SHA,
		changedPaths: [],
		commandRunner: edge.run,
	});

	assert.equal(result.action, "close-existing");
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "commit").length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 1);
	assert.ok(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[1] === "close").length === 1);
	assert.ok(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push" && call.args[2] === "--delete").length === 1);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[1] === "merge").length, 0);
});

test("F-02 controlled edge publishes one leased bot commit and exact-head auto-merge request", () => {
	const changedPaths = [SHARED_BASELINE_PATHS[0], SHARED_BASELINE_PATHS[8]];
	const edge = createControlledCommandEdge({
		stagedPaths: changedPaths,
		pullRequestLists: [[], []],
		metadata: { files: changedPaths },
	});
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths,
		sourceRunUrl: "https://github.example/actions/runs/42",
		commandRunner: edge.run,
	});

	assert.equal(result.action, "merge-requested");
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "commit").length, 1);
	const addCall = edge.calls.find((call) => call.command === "git" && call.args[0] === "add");
	assert.deepEqual(addCall.args.slice(2), SHARED_BASELINE_PATHS);
	const pushCall = edge.calls.find((call) => call.command === "git" && call.args[0] === "push");
	assert.deepEqual(pushCall.args, ["push", "--force-with-lease", "origin", `${SHARED_BASELINE_BOT_BRANCH}:${SHARED_BASELINE_BOT_BRANCH}`]);
	const mergeCall = edge.calls.find((call) => call.command === "gh" && call.args[1] === "merge");
	assert.deepEqual(mergeCall.args.slice(-2), ["--match-head-commit", GENERATED_SHA]);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[1] === "create").length, 1);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[1] === "edit").length, 0);
});

test("F-03 controlled edge supersedes stale main before any publication mutation", () => {
	const edge = createControlledCommandEdge({ currentMainSha: SHA("d") });
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [SHARED_BASELINE_PATHS[0]],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: edge.run,
	});

	assert.equal(result.action, "superseded");
	assert.match(result.reason, new RegExp(SHA("d")));
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh").length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && ["add", "commit", "push"].includes(call.args[0])).length, 0);
});

test("F-04 compares every allowlisted path and reuses an exact existing candidate", () => {
	const changedPaths = [SHARED_BASELINE_PATHS[2]];
	const edge = createControlledCommandEdge({
		remotePaths: changedPaths,
		matchingPaths: SHARED_BASELINE_PATHS,
		pullRequestLists: [[{ number: 42, url: "https://github.example/pull/42" }], [{ number: 42, url: "https://github.example/pull/42" }]],
		generatedSha: BOT_BRANCH_SHA,
		metadata: { files: changedPaths, headRefOid: BOT_BRANCH_SHA },
	});
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		botBranchExists: true,
		botBranchSha: BOT_BRANCH_SHA,
		changedPaths,
		sourceRunUrl: "https://github.example/actions/runs/42",
		commandRunner: edge.run,
	});

	assert.equal(result.action, "merge-requested");
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "diff" && call.args[1] === "--quiet").length, SHARED_BASELINE_PATHS.length);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "commit").length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "reset").length, 1);
	const mergeCall = edge.calls.find((call) => call.command === "gh" && call.args[1] === "merge");
	assert.deepEqual(mergeCall.args.slice(-2), ["--match-head-commit", BOT_BRANCH_SHA]);
});

test("F-05 rejects invalid candidate and remote paths before any later publication action", () => {
	const invalidPath = "docs/internal/baselines/backend-package-file-count.json";
	const candidateEdge = createControlledCommandEdge();
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [invalidPath],
			commandRunner: candidateEdge.run,
		}),
		/unexpected path\(s\).*backend-package-file-count/,
	);
	assert.equal(candidateEdge.calls.length, 0);

	const remoteEdge = createControlledCommandEdge({ remotePaths: [invalidPath] });
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			botBranchExists: true,
			botBranchSha: BOT_BRANCH_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: remoteEdge.run,
		}),
		/unexpected path\(s\).*backend-package-file-count/,
	);
	assert.equal(callsMatching(remoteEdge.calls, (call) => call.command === "gh").length, 0);
	assert.equal(callsMatching(remoteEdge.calls, (call) => call.command === "git" && ["add", "commit", "push"].includes(call.args[0])).length, 0);
});

test("F-12 supersedes a candidate when main moves before publication", () => {
	const edge = createControlledCommandEdge({ currentMainSha: [MAIN_SHA, SHA("d")] });
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [SHARED_BASELINE_PATHS[0]],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: edge.run,
	});

	assert.equal(result.action, "superseded");
	assert.match(result.reason, /candidate publication/);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && ["commit", "push"].includes(call.args[0])).length, 0);
});

test("F-14 rejects duplicate bot PRs before branch publication", () => {
	const edge = createControlledCommandEdge({
		pullRequestLists: [[{ number: 1 }, { number: 2 }]],
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: edge.run,
		}),
		/found 2 open pull requests/,
	);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && ["add", "commit", "push"].includes(call.args[0])).length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && ["create", "edit", "merge"].includes(call.args[1])).length, 0);
});

test("F-16 and F-17 stop before auto-merge for invalid metadata or a failed mutation command", () => {
	const invalidMetadataEdge = createControlledCommandEdge({
		pullRequestLists: [[], []],
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		metadata: { files: [SHARED_BASELINE_PATHS[0]], baseRefName: "develop" },
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: "https://github.example/actions/runs/42",
			commandRunner: invalidMetadataEdge.run,
		}),
		/does not target main/,
	);
	assert.equal(callsMatching(invalidMetadataEdge.calls, (call) => call.command === "gh" && ["ready", "merge"].includes(call.args[1])).length, 0);

	const failedPushEdge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		failWhen: (call) => call.command === "git" && call.args[0] === "push",
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: failedPushEdge.run,
		}),
		/git command failed with exit code 1/,
	);
	assert.equal(callsMatching(failedPushEdge.calls, (call) => call.command === "gh" && ["create", "edit", "merge"].includes(call.args[1])).length, 0);
});
