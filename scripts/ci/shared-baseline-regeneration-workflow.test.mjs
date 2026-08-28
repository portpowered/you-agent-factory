import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
	existsSync,
	mkdtempSync,
	readFileSync,
	rmSync,
	writeFileSync,
} from "node:fs";
import { dirname, join } from "node:path";
import { tmpdir } from "node:os";
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
const makeCommand = process.platform === "win32" ? "make.exe" : "make";

function workflowRunScript(stepName) {
	const stepStart = workflow.indexOf("      - name: " + stepName);
	assert.notEqual(stepStart, -1, "workflow step is missing: " + stepName);
	const nextStep = workflow.indexOf("\n      - name: ", stepStart + 1);
	const step = workflow.slice(stepStart, nextStep === -1 ? workflow.length : nextStep);
	const runStart = step.indexOf("\n        run: |\n");
	assert.notEqual(runStart, -1, "workflow step has no bash script: " + stepName);
	return step
		.slice(runStart + "\n        run: |\n".length)
		.split(/\r?\n/)
		.map((line) => (line.startsWith("          ") ? line.slice(10) : line))
		.join("\n")
		.trimEnd();
}

function bashPath(path) {
	if (process.platform !== "win32") return path;
	const normalized = path.replaceAll("\\", "/");
	return "/mnt/" + normalized[0].toLowerCase() + normalized.slice(2);
}

function bashQuote(value) {
	return "'" + String(value).replaceAll("'", "'\"'\"'") + "'";
}

function runBashScript(script, { cwd, env } = {}) {
	return spawnSync(
		"bash",
		["--noprofile", "--norc", "-e", "-u", "-o", "pipefail"],
		{
			cwd: repositoryRoot,
			input: "cd " + bashQuote(bashPath(cwd)) + "\n" + script + "\n",
			env: { ...process.env, ...env },
			encoding: "utf8",
			windowsHide: true,
		},
	);
}

function bashEnvironment(values) {
	return Object.entries(values)
		.map(([name, value]) => "export " + name + "=" + bashQuote(value))
		.join("\n");
}

function sourceValidationEnvironment(conclusion) {
	return {
		REPOSITORY,
		DEFAULT_BRANCH: "main",
		SOURCE_WORKFLOW_NAME: "CI",
		SOURCE_EVENT: "push",
		SOURCE_REPOSITORY: REPOSITORY,
		SOURCE_HEAD_BRANCH: "main",
		SOURCE_CONCLUSION: conclusion,
		SOURCE_RUN_ID: "42",
		SOURCE_SHA: MAIN_SHA,
		GH_TOKEN: "test-token",
		WRITER_MARKER: "writer-marker",
		GITHUB_MUTATION_MARKER: "github-mutation-marker",
	};
}

function runFakeRegeneration({ failAt = "", failExit = 17 } = {}) {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-writer-"));
	const logPath = join(temporaryDirectory, "writer.log");
	const outputPath = join(temporaryDirectory, "partial-snapshot.txt");
	const fakeGoPath = join(temporaryDirectory, "fake-go.mjs");
	writeFileSync(
		fakeGoPath,
		[
			'import { appendFileSync, writeFileSync } from "node:fs";',
			"const args = process.argv.slice(2);",
			"const stage = args[0] === \"run\" ? args[1].replace(/^\\.\\/cmd\\//, \"\") : args[0] === \"test\" ? args[1].replace(/^\\.\\/pkg\\/transports\\/cli\\//, \"\") : \"unknown\";",
			"appendFileSync(process.env.FAKE_WRITER_LOG, stage + \"\\n\");",
			"if (stage === process.env.FAKE_WRITER_FAIL_AT) {",
			"\tprocess.stderr.write(\"simulated writer failure at \" + stage + \"\\n\");",
			"\tprocess.exit(Number(process.env.FAKE_WRITER_EXIT || \"17\"));",
			"}",
			'if (stage === "unitlanebudget" && process.env.FAKE_WRITER_OUTPUT) writeFileSync(process.env.FAKE_WRITER_OUTPUT, "partial snapshot\\n");',
		].join("\n"),
		"utf8",
	);
	const result = spawnSync(
		makeCommand,
		[
			"regenerate-shared-ci-baselines",
			"BASELINE_REGEN_ROOT=" + temporaryDirectory,
			"UNIT_LATENCY_BUDGET=budget.json",
			"UNIT_LATENCY_SAMPLES=sample-1.json,sample-2.json,sample-3.json",
			"GO=node " + fakeGoPath,
		],
		{
			cwd: repositoryRoot,
			env: {
				...process.env,
				FAKE_WRITER_EXIT: String(failExit),
				FAKE_WRITER_FAIL_AT: failAt,
				FAKE_WRITER_LOG: logPath,
				FAKE_WRITER_OUTPUT: outputPath,
			},
			encoding: "utf8",
			windowsHide: true,
		},
	);
	return {
		temporaryDirectory,
		outputPath,
		result,
		stages: existsSync(logPath) ? readFileSync(logPath, "utf8").trim().split(/\r?\n/).filter(Boolean) : [],
	};
}

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
	assert.match(workflow, /SOURCE_WORKFLOW_NAME: \$\{\{ github\.event\.workflow_run\.name \}\}/);
	assert.match(workflow, /SOURCE_HEAD_BRANCH: \$\{\{ github\.event\.workflow_run\.head_branch \}\}/);
	assert.match(workflow, /SOURCE_CONCLUSION: \$\{\{ github\.event\.workflow_run\.conclusion \}\}/);
	assert.match(workflow, /SOURCE_EVENT\" != \"push\"/);
	assert.match(workflow, /SOURCE_REPOSITORY\" != \"\$REPOSITORY\"/);
	assert.match(workflow, /SOURCE_CONCLUSION\" != \"success\" && \"\$SOURCE_CONCLUSION\" != \"failure\"/);
	assert.match(workflow, /Unsupported source conclusion/);
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
	assert.ok(
		workflow.indexOf("name: Validate the completed source CI identity") <
			workflow.indexOf("name: Check out the delivered main revision"),
		"the source conclusion must be validated before checkout",
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
		...["cancelled", "timed_out", "action_required", "stale", "neutral", "skipped", ""].map(
			(conclusion) => ({ workflowName: "CI", headBranch: "main", conclusion }),
		),
	]) {
		const selection = selectSourceWorkflowRun(input);
		assert.equal(selection.selected, false);
		if (input.workflowName === "CI" && input.headBranch === "main") {
			assert.match(selection.reason, /only success or failure/);
		}
	}
});

test("F-12 rejects every unsupported source conclusion before writer or GitHub mutation", () => {
	const script = workflowRunScript("Validate the completed source CI identity");
	const rejectedConclusions = ["cancelled", "timed_out", "action_required", "stale", "neutral", "skipped", ""];
	for (const conclusion of rejectedConclusions) {
		const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-source-"));
		try {
			const result = runBashScript(
				bashEnvironment(sourceValidationEnvironment(conclusion)) +
					"\n" +
					script +
					'\n: > "$WRITER_MARKER"\n: > "$GITHUB_MUTATION_MARKER"',
				{
					cwd: temporaryDirectory,
				},
			);
			assert.notEqual(result.status, 0, "unsupported source conclusion should stop the job");
			assert.match(result.stderr, /Unsupported source conclusion/);
			assert.equal(existsSync(join(temporaryDirectory, "writer-marker")), false);
			assert.equal(existsSync(join(temporaryDirectory, "github-mutation-marker")), false);
		} finally {
			rmSync(temporaryDirectory, { recursive: true, force: true });
		}
	}

	const eligible = mkdtempSync(join(tmpdir(), "shared-baseline-source-"));
	try {
		const result = runBashScript(
			bashEnvironment(sourceValidationEnvironment("failure")) +
				"\n" +
				script +
				'\n: > "$WRITER_MARKER"\n: > "$GITHUB_MUTATION_MARKER"',
			{
				cwd: eligible,
			},
		);
		assert.equal(result.status, 0, result.stderr);
		assert.equal(existsSync(join(eligible, "writer-marker")), true);
		assert.equal(existsSync(join(eligible, "github-mutation-marker")), true);
	} finally {
		rmSync(eligible, { recursive: true, force: true });
	}
});

test("F-13 empty App token fails before branch or pull-request mutation", () => {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-token-"));
	try {
		const result = runBashScript(
			bashEnvironment({
				BOT_TOKEN: "",
				GITHUB_ENV: "/dev/null",
				WRITER_MARKER: "writer-marker",
				GITHUB_MUTATION_MARKER: "github-mutation-marker",
			}) +
				"\n" +
				workflowRunScript("Export the bot credential for later steps") +
				'\n: > "$WRITER_MARKER"\n: > "$GITHUB_MUTATION_MARKER"',
			{
				cwd: temporaryDirectory,
			},
		);
		assert.notEqual(result.status, 0);
		assert.match(result.stderr, /Missing bot credential/);
		assert.equal(existsSync(join(temporaryDirectory, "writer-marker")), false);
		assert.equal(existsSync(join(temporaryDirectory, "github-mutation-marker")), false);
		assert.doesNotMatch(result.stdout + result.stderr, /gh (pr|api)|git push/);
	} finally {
		rmSync(temporaryDirectory, { recursive: true, force: true });
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

test("F-05/F-15 parses status output and rejects every path outside the eleven-file allowlist", () => {
	const status = [
		...SHARED_BASELINE_PATHS.map((path) => ` M ${path}`),
		`R  old.txt -> ${SHARED_BASELINE_PATHS[0]}`,
	].join("\n");
	assert.deepEqual(
		parsePorcelainPaths(status),
		["old.txt", ...SHARED_BASELINE_PATHS].sort(),
	);
	assert.throws(
		() => validateAllowlistedPaths(parsePorcelainPaths("R  old.txt -> " + SHARED_BASELINE_PATHS[0])),
		/unexpected path\(s\).*old\.txt/,
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

test("F-06/F-07 stop the integrated writer spine and never hand off a partial candidate", () => {
	const firstFailure = runFakeRegeneration({
		failAt: "ownershipinventoryfreeze",
		failExit: 17,
	});
	try {
		assert.notEqual(firstFailure.result.status, 0, firstFailure.result.stdout + firstFailure.result.stderr);
		assert.deepEqual(firstFailure.stages, ["unitlanebudget", "ownershipinventoryfreeze"]);
		assert.equal(readFileSync(firstFailure.outputPath, "utf8"), "partial snapshot\n");
		assert.doesNotMatch(firstFailure.result.stdout + firstFailure.result.stderr, /mcptoolinventorygen|publication succeeded/);
		const plan = planReconciliation({
			triggeringSha: MAIN_SHA,
			currentMainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			generationError: "ownershipinventoryfreeze exited 17",
		});
		assert.equal(plan.action, "fail");
		assert.equal(plan.publish, false);
	} finally {
		rmSync(firstFailure.temporaryDirectory, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 });
	}

	const laterFailure = runFakeRegeneration({
		failAt: "cliinputs",
		failExit: 23,
	});
	try {
		assert.notEqual(laterFailure.result.status, 0, laterFailure.result.stdout + laterFailure.result.stderr);
		assert.deepEqual(laterFailure.stages, [
			"unitlanebudget",
			"ownershipinventoryfreeze",
			"commandidentity",
			"cliinputs",
		]);
		assert.equal(readFileSync(laterFailure.outputPath, "utf8"), "partial snapshot\n");
		assert.doesNotMatch(laterFailure.result.stdout + laterFailure.result.stderr, /mcptoolinventorygen|publication succeeded/);
	} finally {
		rmSync(laterFailure.temporaryDirectory, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 });
	}

	const cleanRetry = runFakeRegeneration();
	try {
		assert.equal(cleanRetry.result.status, 0, cleanRetry.result.stdout + cleanRetry.result.stderr);
		assert.deepEqual(cleanRetry.stages, [
			"unitlanebudget",
			"ownershipinventoryfreeze",
			"commandidentity",
			"cliinputs",
			"mcptoolinventorygen",
		]);
	} finally {
		rmSync(cleanRetry.temporaryDirectory, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 });
	}
});

test("F-18 keeps the newest overlapping run as the only publisher", () => {
	const olderEdge = createControlledCommandEdge({ currentMainSha: SHA("d") });
	const older = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [SHARED_BASELINE_PATHS[0]],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: olderEdge.run,
	});
	assert.equal(older.action, "superseded");
	assert.equal(callsMatching(olderEdge.calls, (call) => call.command === "git" && ["add", "commit", "push"].includes(call.args[0])).length, 0);
	assert.equal(callsMatching(olderEdge.calls, (call) => call.command === "gh").length, 0);

	const newerEdge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		pullRequestLists: [[], []],
		metadata: { files: [SHARED_BASELINE_PATHS[0]] },
	});
	const newer = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [SHARED_BASELINE_PATHS[0]],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: newerEdge.run,
	});
	assert.equal(newer.action, "merge-requested");
	assert.equal(callsMatching(newerEdge.calls, (call) => call.command === "gh" && call.args[1] === "merge").length, 1);
});

test("F-19 cancellation during mutation stops terminal success and permits a clean retry", () => {
	const cancellation = new Error("cancelled by newer run");
	cancellation.name = "AbortError";
	const cancelledEdge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		failWhen: (call) => call.command === "git" && call.args[0] === "push" ? cancellation : undefined,
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: cancelledEdge.run,
		}),
		/cancelled by newer run/,
	);
	assert.equal(callsMatching(cancelledEdge.calls, (call) => call.command === "gh" && ["create", "edit", "merge"].includes(call.args[1])).length, 0);

	const retryEdge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		pullRequestLists: [[], []],
		metadata: { files: [SHARED_BASELINE_PATHS[0]] },
	});
	const retry = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [SHARED_BASELINE_PATHS[0]],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: retryEdge.run,
	});
	assert.equal(retry.action, "merge-requested");
	assert.equal(callsMatching(retryEdge.calls, (call) => call.command === "gh" && call.args[1] === "merge").length, 1);
});

test("F-20 capacity failure returns once without a publication retry", () => {
	const edge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		failWhen: (call) => call.command === "git" && call.args[0] === "commit" ? 28 : undefined,
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: edge.run,
		}),
		/git command failed with exit code 28/,
	);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "commit").length, 1);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 0);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && ["create", "edit", "merge"].includes(call.args[1])).length, 0);
});

test("F-23 loopback reports a blocked generation with a delta plan and does not repair it", () => {
	const failed = runFakeRegeneration({
		failAt: "ownershipinventoryfreeze",
		failExit: 28,
	});
	try {
		const beforeReport = readFileSync(failed.outputPath, "utf8");
		const plan = planReconciliation({
			triggeringSha: MAIN_SHA,
			currentMainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			generationError: "ownershipinventoryfreeze exited 28 (capacity)",
		});
		const report = {
			verdict: plan.publish ? "PASS" : "BLOCKED",
			evidence: plan.reason,
			deltaPlan: "repair the failing writer and rerun the target; do not publish the partial candidate",
		};
		assert.notEqual(failed.result.status, 0);
		assert.equal(report.verdict, "BLOCKED");
		assert.match(report.evidence, /generation failed before publication/);
		assert.match(report.deltaPlan, /do not publish/);
		assert.equal(readFileSync(failed.outputPath, "utf8"), beforeReport);
	} finally {
		rmSync(failed.temporaryDirectory, { recursive: true, force: true, maxRetries: 3, retryDelay: 100 });
	}
});
