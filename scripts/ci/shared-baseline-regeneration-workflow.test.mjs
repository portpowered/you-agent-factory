import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import {
	existsSync,
	mkdtempSync,
	mkdirSync,
	readdirSync,
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
	SHARED_BASELINE_UNIT_BUDGET_PATH,
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
const NODE_DEFAULT_CHILD_OUTPUT_BUFFER_BYTES = 1024 * 1024;
const OVERSIZED_TEST_INVENTORY_COUNT = 20_000;

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
	remoteBaseSha = MAIN_SHA,
	remoteParentSha = remoteBaseSha,
	remoteHeadSha = BOT_BRANCH_SHA,
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
			if (args[0] === "rev-parse" && args[1] === "origin/" + SHARED_BASELINE_BOT_BRANCH) {
				return { status: 0, stdout: `${remoteHeadSha}\n` };
			}
			if (args[0] === "rev-parse" && args[1] === "origin/" + SHARED_BASELINE_BOT_BRANCH + "^") {
				return { status: 0, stdout: `${remoteParentSha}\n` };
			}
			if (args[0] === "merge-base") {
				return { status: 0, stdout: `${remoteBaseSha}\n` };
			}
			if (args[0] === "rev-parse" && args[1] === "HEAD") {
				return { status: 0, stdout: `${generatedSha}\n` };
			}
			if (args[0] === "rev-parse" && args[1] === "HEAD^") {
				return { status: 0, stdout: `${MAIN_SHA}\n` };
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

const LOCAL_GIT_SNAPSHOT_PATH = SHARED_BASELINE_PATHS[0];
const LOCAL_GIT_MAIN_ONLY_PATH = "docs/internal/baselines/newer-main-only.txt";

function runLocalGit(cwd, args, { env = {} } = {}) {
	const result = spawnSync("git", args, {
		cwd,
		env: { ...process.env, GIT_CONFIG_NOSYSTEM: "1", ...env },
		encoding: "utf8",
		windowsHide: true,
	});
	return {
		status: result.status ?? -1,
		stdout: result.stdout || "",
		stderr: result.error ? `${result.stderr || ""}${result.error.message}` : result.stderr || "",
	};
}

function requireLocalGit(cwd, args, options = {}) {
	const result = runLocalGit(cwd, args, options);
	assert.equal(
		result.status,
		0,
		`git ${args.join(" ")} failed: ${result.stderr.trim()}`,
	);
	return result.stdout.trim();
}

function writeLocalGitFile(repository, path, contents) {
	const filePath = join(repository, path);
	mkdirSync(dirname(filePath), { recursive: true });
	writeFileSync(filePath, contents, "utf8");
}

function createWorkingTreeValidationFixture({
	sourcePath = LOCAL_GIT_SNAPSHOT_PATH,
	candidate = "identity",
	staged = false,
	sourceTopology = "single-parent",
	sourceOperation = "modify",
	sourceDestinationPath = "docs/internal/baselines/source-renamed-or-copied.txt",
	oversized = false,
} = {}) {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-validation-"));
	const repository = join(temporaryDirectory, "repository");
	const githubOutput = join(temporaryDirectory, "github-output.txt");
	const blobTemporaryDirectory = join(temporaryDirectory, "blob-tmp");
	const currentBudget = {
		version: 1,
		owner: "backend-unit-lane",
		reference: {
			baseCommit: SHA("a"),
			runnerImage: "ubuntu-24.04",
			...(oversized
				? {
					testInventory: Array.from(
						{ length: OVERSIZED_TEST_INVENTORY_COUNT },
						(_, index) =>
							`github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime::TestOversizedBudget/${String(index).padStart(5, "0")}`,
					),
				}
				: {}),
		},
	};

	try {
		requireLocalGit(temporaryDirectory, ["init", repository]);
		requireLocalGit(repository, ["config", "user.name", "shared-baseline-validation"]);
		requireLocalGit(repository, ["config", "user.email", "shared-baseline-validation@example.invalid"]);
		mkdirSync(blobTemporaryDirectory, { recursive: true });
		writeLocalGitFile(repository, SHARED_BASELINE_UNIT_BUDGET_PATH, JSON.stringify(currentBudget, null, 2) + "\n");
		writeLocalGitFile(repository, sourcePath, "before source change\n");
		requireLocalGit(repository, ["add", "--", SHARED_BASELINE_UNIT_BUDGET_PATH, sourcePath]);
		requireLocalGit(repository, ["commit", "-m", "validation fixture base"]);
		requireLocalGit(repository, ["branch", "-M", "main"]);
		if (sourceTopology === "single-parent") {
			if (sourceOperation === "modify") {
				writeLocalGitFile(repository, sourcePath, "after source change\n");
				requireLocalGit(repository, ["add", "--", sourcePath]);
			} else if (sourceOperation === "rename") {
				requireLocalGit(repository, ["mv", sourcePath, sourceDestinationPath]);
			} else if (sourceOperation === "copy") {
				writeLocalGitFile(
					repository,
					sourceDestinationPath,
					readFileSync(join(repository, sourcePath), "utf8"),
				);
				requireLocalGit(repository, ["add", "--", sourceDestinationPath]);
			} else {
				throw new Error(`unsupported validation fixture source operation: ${sourceOperation}`);
			}
			requireLocalGit(repository, ["commit", "-m", "validation fixture source"]);
		} else if (sourceTopology === "two-parent-merge") {
			if (sourceOperation !== "modify") {
				throw new Error(`source operation ${sourceOperation} is not supported for a two-parent merge fixture`);
			}
			requireLocalGit(repository, ["switch", "-c", "source-change"]);
			writeLocalGitFile(repository, sourcePath, "after source change\n");
			requireLocalGit(repository, ["add", "--", sourcePath]);
			requireLocalGit(repository, ["commit", "-m", "validation fixture source branch"]);
			requireLocalGit(repository, ["switch", "main"]);
			requireLocalGit(repository, ["merge", "--no-ff", "source-change", "-m", "validation fixture merge"]);
		} else {
			throw new Error(`unsupported validation fixture source topology: ${sourceTopology}`);
		}
		const sourceSha = requireLocalGit(repository, ["rev-parse", "HEAD"]);

		const candidateBudget = structuredClone(currentBudget);
		if (candidate === "identity") {
			candidateBudget.reference.baseCommit = sourceSha;
		} else if (candidate === "material") {
			candidateBudget.reference.baseCommit = sourceSha;
			candidateBudget.reference.runnerImage = "ubuntu-25.04";
		} else if (candidate === "malformed") {
			writeLocalGitFile(repository, SHARED_BASELINE_UNIT_BUDGET_PATH, "{ malformed\n");
		} else {
			throw new Error(`unsupported validation fixture candidate: ${candidate}`);
		}
		if (candidate !== "malformed") {
			writeLocalGitFile(repository, SHARED_BASELINE_UNIT_BUDGET_PATH, JSON.stringify(candidateBudget, null, 2) + "\n");
		}
		if (staged) requireLocalGit(repository, ["add", "--", SHARED_BASELINE_UNIT_BUDGET_PATH]);

		return {
			temporaryDirectory,
			repository,
			githubOutput,
			blobTemporaryDirectory,
			sourceSha,
			currentBudgetContents: JSON.stringify(currentBudget, null, 2) + "\n",
			cleanup() {
				rmSync(temporaryDirectory, { recursive: true, force: true });
			},
		};
	} catch (error) {
		rmSync(temporaryDirectory, { recursive: true, force: true });
		throw error;
	}
}

function runWorkingTreeValidation(fixture, extraArguments = [], { sourceSha = fixture.sourceSha, env = {} } = {}) {
	return spawnSync(
		process.execPath,
		[
			helper,
			"validate-working-tree",
			"--source-sha",
			sourceSha,
			"--github-output",
			fixture.githubOutput,
			...extraArguments,
		],
		{
			cwd: fixture.repository,
			env: {
				...process.env,
				GIT_CONFIG_NOSYSTEM: "1",
				TMPDIR: fixture.blobTemporaryDirectory,
				TMP: fixture.blobTemporaryDirectory,
				TEMP: fixture.blobTemporaryDirectory,
				...env,
			},
			encoding: "utf8",
			windowsHide: true,
		},
	);
}

function createLocalGitFixture({ botBase = "stale", onTemporaryDirectory } = {}) {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-git-"));
	onTemporaryDirectory?.(temporaryDirectory);
	const originDirectory = join(temporaryDirectory, "origin.git");
	const sourceDirectory = join(temporaryDirectory, "source");
	const runnerDirectory = join(temporaryDirectory, "runner");

	try {
		if (botBase !== "stale" && botBase !== "current") {
			throw new Error(`unsupported local Git fixture bot base: ${botBase}`);
		}
		requireLocalGit(temporaryDirectory, ["init", "--bare", originDirectory]);
		requireLocalGit(temporaryDirectory, ["init", sourceDirectory]);
		requireLocalGit(sourceDirectory, ["config", "user.name", "shared-baseline-test"]);
		requireLocalGit(sourceDirectory, ["config", "user.email", "shared-baseline-test@example.invalid"]);
		for (const path of SHARED_BASELINE_PATHS) {
			writeLocalGitFile(sourceDirectory, path, `baseline ${path}\n`);
		}
		requireLocalGit(sourceDirectory, ["add", "--", ...SHARED_BASELINE_PATHS]);
		requireLocalGit(sourceDirectory, ["commit", "-m", "base snapshot"]);
		requireLocalGit(sourceDirectory, ["branch", "-M", "main"]);
		requireLocalGit(sourceDirectory, ["remote", "add", "origin", originDirectory]);
		requireLocalGit(sourceDirectory, ["push", "origin", "main"]);
		requireLocalGit(originDirectory, ["symbolic-ref", "HEAD", "refs/heads/main"]);
		const baseSha = requireLocalGit(sourceDirectory, ["rev-parse", "HEAD"]);

		if (botBase === "stale") {
			requireLocalGit(sourceDirectory, ["checkout", "-b", "stale-bot", baseSha]);
			writeLocalGitFile(sourceDirectory, LOCAL_GIT_SNAPSHOT_PATH, "generated snapshot\n");
			requireLocalGit(sourceDirectory, ["add", "--", LOCAL_GIT_SNAPSHOT_PATH]);
			requireLocalGit(sourceDirectory, ["commit", "-m", "generated snapshot"]);
			requireLocalGit(sourceDirectory, ["push", "origin", `HEAD:${SHARED_BASELINE_BOT_BRANCH}`]);
		}

		requireLocalGit(sourceDirectory, ["checkout", "main"]);
		writeLocalGitFile(sourceDirectory, LOCAL_GIT_MAIN_ONLY_PATH, "newer main content\n");
		requireLocalGit(sourceDirectory, ["add", "--", LOCAL_GIT_MAIN_ONLY_PATH]);
		requireLocalGit(sourceDirectory, ["commit", "-m", "unrelated newer main change"]);
		const mainSha = requireLocalGit(sourceDirectory, ["rev-parse", "HEAD"]);
		requireLocalGit(sourceDirectory, ["push", "origin", "main"]);

		if (botBase === "current") {
			requireLocalGit(sourceDirectory, ["checkout", "-b", "current-bot", mainSha]);
			writeLocalGitFile(sourceDirectory, LOCAL_GIT_SNAPSHOT_PATH, "generated snapshot\n");
			requireLocalGit(sourceDirectory, ["add", "--", LOCAL_GIT_SNAPSHOT_PATH]);
			requireLocalGit(sourceDirectory, ["commit", "-m", "generated snapshot"]);
			requireLocalGit(sourceDirectory, ["push", "origin", `HEAD:${SHARED_BASELINE_BOT_BRANCH}`]);
		}

		const botBranchSha = requireLocalGit(sourceDirectory, ["rev-parse", `origin/${SHARED_BASELINE_BOT_BRANCH}`]);
		requireLocalGit(temporaryDirectory, ["clone", "--branch", "main", originDirectory, runnerDirectory]);
		requireLocalGit(runnerDirectory, ["checkout", "--detach", mainSha]);
		requireLocalGit(runnerDirectory, ["switch", "-C", SHARED_BASELINE_BOT_BRANCH, mainSha]);
		writeLocalGitFile(runnerDirectory, LOCAL_GIT_SNAPSHOT_PATH, "generated snapshot\n");

		return {
			temporaryDirectory,
			sourceDirectory,
			runnerDirectory,
			baseSha,
			mainSha,
			botBranchSha,
			cleanup() {
				rmSync(temporaryDirectory, { recursive: true, force: true });
			},
		};
	} catch (error) {
		rmSync(temporaryDirectory, { recursive: true, force: true });
		throw error;
	}
}

function createLocalGitCommandEdge({
	cwd,
	botBranchSha,
	changedPaths = [],
	pullRequestLists = [[]],
	metadata = {},
	failWhen,
} = {}) {
	const calls = [];
	let pullRequestListReads = 0;
	const pullRequestMetadata = {
		baseRefName: "main",
		headRefName: SHARED_BASELINE_BOT_BRANCH,
		headRefOid: botBranchSha,
		isDraft: false,
		files: changedPaths.map((path) => ({ path })),
		...metadata,
	};
	if (Array.isArray(pullRequestMetadata.files)) {
		pullRequestMetadata.files = pullRequestMetadata.files.map((file) =>
			typeof file === "string" ? { path: file } : file,
		);
	}
	const hasExplicitHeadRefOid = Object.prototype.hasOwnProperty.call(metadata, "headRefOid");
	let generatedCandidateCommitted = false;

	function run(command, args, options = {}) {
		const call = { command, args: [...args], options: { ...options } };
		calls.push(call);
		const failure = failWhen?.(call);
		if (failure) {
			if (failure instanceof Error) throw failure;
			return {
				status: typeof failure === "number" ? failure : 1,
				stderr: "simulated command failure",
			};
		}
		if (command === "git") {
			const result = runLocalGit(cwd, args);
			if (args[0] === "commit" && result.status === 0) generatedCandidateCommitted = true;
			return result;
		}
		if (command !== "gh") return { status: 0, stdout: "" };
		if (args[0] === "auth" && args[1] === "setup-git") return { status: 0, stdout: "" };
		if (args[0] === "pr" && args[1] === "list") {
			const index = Math.min(pullRequestListReads, pullRequestLists.length - 1);
			pullRequestListReads += 1;
			return { status: 0, stdout: JSON.stringify(pullRequestLists[index]) };
		}
		if (args[0] === "pr" && args[1] === "create") {
			return { status: 0, stdout: "https://github.example/pull/42\n" };
		}
		if (args[0] === "pr" && args[1] === "view" && args.includes("--jq")) {
			return { status: 0, stdout: "42\n" };
		}
		if (args[0] === "pr" && args[1] === "view") {
			const headRefOid = hasExplicitHeadRefOid
				? pullRequestMetadata.headRefOid
				: generatedCandidateCommitted
					? requireLocalGit(cwd, ["rev-parse", "HEAD"])
					: botBranchSha;
			return { status: 0, stdout: JSON.stringify({ ...pullRequestMetadata, headRefOid }) };
		}
		return { status: 0, stdout: "" };
	}

	return { calls, run };
}

function captureLocalRepositoryState(cwd) {
	return {
		head: requireLocalGit(cwd, ["rev-parse", "HEAD"]),
		status: requireLocalGit(cwd, ["status", "--porcelain=v1", "--untracked-files=all"]),
		refs: requireLocalGit(cwd, ["for-each-ref", "--format=%(refname)=%(objectname)", "refs/heads", "refs/remotes"]),
	};
}

const AUTH_DEFAULT_TOKEN = "default-fixture-token";
const AUTH_APP_TOKEN = "app-fixture-token";

function gitShellPath(path) {
	return process.platform === "win32" ? path.replaceAll("\\", "/") : path;
}

function credentialHelperCommand({ label, token, tracePath }) {
	const trace = bashQuote(gitShellPath(tracePath));
	return [
		"!f() { case \"\${1:-}\" in get)",
		`printf '%s\\n' ${bashQuote(label)} >> ${trace};`,
		`printf '%s\\n' ${bashQuote(`username=${label}`)};`,
		`printf '%s\\n' ${bashQuote(`password=${token}`)};`,
		";; *) exit 0 ;; esac; }; f",
	].join(" ");
}

function startCredentialWitnessServer(temporaryDirectory) {
	const tracePath = join(temporaryDirectory, "credential-trace.log");
	const serverScript = join(temporaryDirectory, "credential-witness-server.mjs");
	writeFileSync(
		serverScript,
		[
			'import { appendFileSync } from "node:fs";',
			'import { createServer } from "node:http";',
			"const tracePath = process.argv[2];",
			"const labels = new Map([[\"default-fixture-token\", \"default\"], [\"app-fixture-token\", \"app\"]]);",
			"const server = createServer((request, response) => {",
			"\tconst authorization = request.headers.authorization || \"\";",
			"\tif (!authorization.startsWith(\"Basic \")) {",
			"\t\tappendFileSync(tracePath, \"anonymous\\n\");",
			"\t\tresponse.writeHead(401, { \"WWW-Authenticate\": \"Basic realm=credential-witness\" });",
			"\t\tresponse.end();",
			"\t\treturn;",
			"\t}",
			"\tconst decoded = Buffer.from(authorization.slice(6), \"base64\").toString(\"utf8\");",
			"\tconst password = decoded.slice(decoded.indexOf(\":\") + 1);",
			"\tappendFileSync(tracePath, (labels.get(password) || \"unknown\") + \"\\n\");",
			"\tresponse.writeHead(200, { \"Content-Type\": \"text/plain\" });",
			"\tresponse.end(\"credential witness response\");",
			"});",
			"server.listen(0, \"127.0.0.1\", () => {",
			"\tprocess.stdout.write(`READY ${server.address().port}\\n`);",
			"});",
		].join("\n"),
		"utf8",
	);

	const child = spawn(process.execPath, [serverScript, tracePath], {
		stdio: ["ignore", "pipe", "pipe"],
		windowsHide: true,
	});
	let output = "";
	let settled = false;
	return new Promise((resolve, reject) => {
		const fail = (error) => {
			if (settled) return;
			settled = true;
			reject(error);
		};
		child.once("error", (error) => fail(new Error(`credential witness server failed to start: ${error.message}`)));
		child.once("exit", (code) => {
			if (code !== 0) fail(new Error(`credential witness server exited before readiness with code ${code}`));
		});
		child.stdout.on("data", (chunk) => {
			output += chunk.toString();
			const ready = output.match(/READY (\d+)/);
			if (!ready || settled) return;
			settled = true;
			resolve({ child, port: Number(ready[1]), tracePath });
		});
	});
}

async function stopCredentialWitnessServer(server) {
	if (!server || server.child.exitCode !== null) return;
	const exited = new Promise((resolve) => server.child.once("exit", resolve));
	server.child.kill();
	await exited;
}

async function createLocalGitCredentialFixture({
	userName = "credential-witness",
	userEmail = "credential-witness@example.invalid",
	onTemporaryDirectory,
	failAt = "",
} = {}) {
	const temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-credential-"));
	onTemporaryDirectory?.(temporaryDirectory);
	let server;
	const globalConfigPath = join(temporaryDirectory, "empty-global.gitconfig");
	const originDirectory = join(temporaryDirectory, "origin.git");
	const runnerDirectory = join(temporaryDirectory, "runner");
	try {
		server = await startCredentialWitnessServer(temporaryDirectory);
		if (failAt === "server") throw new Error("simulated credential witness setup failure");
		writeFileSync(globalConfigPath, "", "utf8");
		const gitOptions = { env: { GIT_CONFIG_GLOBAL: globalConfigPath } };
		const run = (cwd, args) => runLocalGit(cwd, args, gitOptions);
		const requireGit = (cwd, args) => {
			const result = run(cwd, args);
			assert.equal(result.status, 0, `git ${args.join(" ")} failed: ${result.stderr.trim()}`);
			return result.stdout.trim();
		};
		requireGit(temporaryDirectory, ["init", "--bare", originDirectory]);
		requireGit(temporaryDirectory, ["init", runnerDirectory]);
		requireGit(runnerDirectory, ["config", "user.name", userName]);
		requireGit(runnerDirectory, ["config", "user.email", userEmail]);
		writeFileSync(join(runnerDirectory, "witness.txt"), "credential witness\n", "utf8");
		requireGit(runnerDirectory, ["add", "--", "witness.txt"]);
		requireGit(runnerDirectory, ["commit", "-m", "credential witness"]);
		const remoteUrl = `http://127.0.0.1:${server.port}/repo.git`;
		requireGit(runnerDirectory, ["remote", "add", "origin", remoteUrl]);
		if (failAt === "git") throw new Error("simulated credential witness Git setup failure");
		return {
			temporaryDirectory,
			runnerDirectory,
			globalConfigPath,
			remoteUrl,
			tracePath: server.tracePath,
			helperTracePath: join(temporaryDirectory, "credential-helper-trace.log"),
			server,
			cleanup: async () => {
				await stopCredentialWitnessServer(server);
				rmSync(temporaryDirectory, { recursive: true, force: true });
			},
		};
	} catch (error) {
		await stopCredentialWitnessServer(server);
		rmSync(temporaryDirectory, { recursive: true, force: true });
		throw error;
	}
}

function configureLocalGitCredentialWitness(fixture, {
	checkoutToken = "",
	helperOrder = ["app", "default"],
} = {}) {
	const gitOptions = { env: { GIT_CONFIG_GLOBAL: fixture.globalConfigPath } };
	const requireGit = (args) => {
		const result = runLocalGit(fixture.runnerDirectory, args, gitOptions);
		assert.equal(result.status, 0, `git ${args.join(" ")} failed: ${result.stderr.trim()}`);
		return result.stdout.trim();
	};
	if (checkoutToken) {
		const encoded = Buffer.from(`checkout:${checkoutToken}`, "utf8").toString("base64");
		requireGit([
			"config",
			"--local",
			`http.${fixture.remoteUrl}.extraheader`,
			`AUTHORIZATION: Basic ${encoded}`,
		]);
	}
	const tokens = { app: AUTH_APP_TOKEN, default: AUTH_DEFAULT_TOKEN };
	for (const label of helperOrder) {
		if (!tokens[label]) throw new Error(`unsupported credential witness helper: ${label}`);
		requireGit([
			"config",
			"--local",
			"--add",
			"credential.helper",
			credentialHelperCommand({
				label,
				token: tokens[label],
				tracePath: fixture.helperTracePath,
			}),
		]);
	}
}

async function runLocalGitCredentialWitness(options = {}) {
	const fixture = await createLocalGitCredentialFixture(options);
	const gitOptions = {
		env: {
			GIT_CONFIG_GLOBAL: fixture.globalConfigPath,
			GIT_TERMINAL_PROMPT: "0",
		},
	};
	try {
		configureLocalGitCredentialWitness(fixture, options);
		const push = runLocalGit(
			fixture.runnerDirectory,
			["push", "origin", "HEAD:main"],
			gitOptions,
		);
		const readLines = (path) => (existsSync(path) ? readFileSync(path, "utf8").trim().split(/\r?\n/).filter(Boolean) : []);
		return {
			push,
			authenticatedLabels: readLines(fixture.tracePath).filter((label) => label !== "anonymous"),
			helperLabels: readLines(fixture.helperTracePath),
			metadata: runLocalGit(fixture.runnerDirectory, ["show", "-s", "--format=%an <%ae>", "HEAD"], gitOptions).stdout.trim(),
			temporaryDirectory: fixture.temporaryDirectory,
		};
	} finally {
		await fixture.cleanup();
	}
}

test("AUTH-01/AUTH-02 local-real Git records App credential identity and characterizes checkout order dependency", async () => {
	const retainedCheckout = await runLocalGitCredentialWitness({
		checkoutToken: AUTH_DEFAULT_TOKEN,
		helperOrder: ["app", "default"],
	});
	const appOnlyCheckout = await runLocalGitCredentialWitness({
		checkoutToken: "",
		helperOrder: ["app"],
	});

	assert.notEqual(retainedCheckout.push.status, 0);
	assert.notEqual(appOnlyCheckout.push.status, 0);
	// Characterization: a checkout-persisted extraheader is selected before the
	// later helper, while disabling persistence lets the App helper win.
	assert.ok(retainedCheckout.authenticatedLabels.length > 0);
	assert.ok(retainedCheckout.authenticatedLabels.every((label) => label === "default"));
	assert.ok(appOnlyCheckout.authenticatedLabels.length > 0);
	assert.ok(appOnlyCheckout.authenticatedLabels.every((label) => label === "app"));
	assert.deepEqual(appOnlyCheckout.helperLabels, ["app"]);
	assert.doesNotMatch(appOnlyCheckout.helperLabels.join("\n"), /default/);
	assert.doesNotMatch(
		`${retainedCheckout.push.stdout}${retainedCheckout.push.stderr}${appOnlyCheckout.push.stdout}${appOnlyCheckout.push.stderr}`,
		/default-fixture-token|app-fixture-token/,
	);
	assert.equal(existsSync(retainedCheckout.temporaryDirectory), false);
	assert.equal(existsSync(appOnlyCheckout.temporaryDirectory), false);
});

test("AUTH-03 local-real Git keeps authentication independent from commit metadata", async () => {
	const githubActionsMetadata = await runLocalGitCredentialWitness({
		checkoutToken: AUTH_DEFAULT_TOKEN,
		helperOrder: ["app", "default"],
		userName: "github-actions[bot]",
		userEmail: "41898282+github-actions[bot]@users.noreply.github.com",
	});
	const appLookingMetadata = await runLocalGitCredentialWitness({
		checkoutToken: AUTH_DEFAULT_TOKEN,
		helperOrder: ["app", "default"],
		userName: "you-baseline-bot[bot]",
		userEmail: "baseline-bot@example.invalid",
	});

	assert.ok(githubActionsMetadata.authenticatedLabels.length > 0);
	assert.ok(githubActionsMetadata.authenticatedLabels.every((label) => label === "default"));
	assert.ok(appLookingMetadata.authenticatedLabels.length > 0);
	assert.ok(appLookingMetadata.authenticatedLabels.every((label) => label === "default"));
	assert.notEqual(githubActionsMetadata.metadata, appLookingMetadata.metadata);
	assert.match(githubActionsMetadata.metadata, /^github-actions\[bot\] </);
	assert.match(appLookingMetadata.metadata, /^you-baseline-bot\[bot\] </);
	assert.doesNotMatch(
		`${githubActionsMetadata.push.stdout}${githubActionsMetadata.push.stderr}${appLookingMetadata.push.stdout}${appLookingMetadata.push.stderr}`,
		/default-fixture-token|app-fixture-token/,
	);
	assert.equal(existsSync(githubActionsMetadata.temporaryDirectory), false);
	assert.equal(existsSync(appLookingMetadata.temporaryDirectory), false);
});

test("F-19 credential witness setup and command failures clean fixtures without changing developer state", async () => {
	const developerState = captureLocalRepositoryState(repositoryRoot);
	let failedSetupDirectory = "";
	await assert.rejects(
		() =>
			createLocalGitCredentialFixture({
				failAt: "server",
				onTemporaryDirectory: (directory) => {
					failedSetupDirectory = directory;
				},
			}),
		/simulated credential witness setup failure/,
	);
	assert.equal(existsSync(failedSetupDirectory), false);

	const fixture = await createLocalGitCredentialFixture();
	try {
		configureLocalGitCredentialWitness(fixture, { helperOrder: ["app"] });
		const gitOptions = {
			env: {
				GIT_CONFIG_GLOBAL: fixture.globalConfigPath,
				GIT_TERMINAL_PROMPT: "0",
			},
		};
		assert.throws(
			() => requireLocalGit(fixture.runnerDirectory, ["push", "origin", "HEAD:main"], gitOptions),
			/failed:/,
		);
	} finally {
		const temporaryDirectory = fixture.temporaryDirectory;
		await fixture.cleanup();
		assert.equal(existsSync(temporaryDirectory), false);
	}
	assert.deepEqual(captureLocalRepositoryState(repositoryRoot), developerState);
});

test("TASK-001 local-real Git characterizes stale bot ancestry without treating newer main paths as branch changes", () => {
	const developerState = captureLocalRepositoryState(repositoryRoot);
	const fixture = createLocalGitFixture({ botBase: "stale" });
	try {
		assert.equal(
			requireLocalGit(fixture.runnerDirectory, ["rev-parse", `origin/${SHARED_BASELINE_BOT_BRANCH}^`]),
			fixture.baseSha,
		);
		assert.notEqual(fixture.baseSha, fixture.mainSha);
		const remotePaths = requireLocalGit(fixture.runnerDirectory, [
			"diff",
			"--name-only",
			fixture.mainSha,
			`origin/${SHARED_BASELINE_BOT_BRANCH}`,
		])
			.split(/\r?\n/)
			.filter(Boolean);
		assert.deepEqual(remotePaths.sort(), [LOCAL_GIT_MAIN_ONLY_PATH, LOCAL_GIT_SNAPSHOT_PATH].sort());
	} finally {
		fixture.cleanup();
	}
	assert.equal(existsSync(fixture.temporaryDirectory), false);
	assert.deepEqual(captureLocalRepositoryState(repositoryRoot), developerState);
});

test("TASK-002 local-real Git reconstructs a stale bot branch from exact main and reuses its PR", () => {
	const developerState = captureLocalRepositoryState(repositoryRoot);
	const fixture = createLocalGitFixture({ botBase: "stale" });
	try {
		const edge = createLocalGitCommandEdge({
			cwd: fixture.runnerDirectory,
			botBranchSha: fixture.botBranchSha,
			changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
			pullRequestLists: [[{ number: 42, url: "https://github.example/pull/42" }], [{ number: 42, url: "https://github.example/pull/42" }]],
			metadata: { files: [LOCAL_GIT_SNAPSHOT_PATH] },
		});
		const result = reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: fixture.mainSha,
			botBranchExists: true,
			botBranchSha: fixture.botBranchSha,
			changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: edge.run,
		});

		assert.equal(result.action, "merge-requested");
		assert.deepEqual(result.remotePaths, [LOCAL_GIT_SNAPSHOT_PATH]);
		const pushCall = edge.calls.find((call) => call.command === "git" && call.args[0] === "push");
		assert.deepEqual(pushCall.args, [
			"push",
			`--force-with-lease=refs/heads/${SHARED_BASELINE_BOT_BRANCH}:${fixture.botBranchSha}`,
			"origin",
			`${SHARED_BASELINE_BOT_BRANCH}:${SHARED_BASELINE_BOT_BRANCH}`,
		]);
		assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[1] === "edit").length, 1);
		const mergeCall = edge.calls.find((call) => call.command === "gh" && call.args[1] === "merge");
		assert.deepEqual(mergeCall.args.slice(-2), ["--match-head-commit", result.commitSha]);

		requireLocalGit(fixture.runnerDirectory, [
			"fetch",
			"--force",
			"origin",
			`refs/heads/${SHARED_BASELINE_BOT_BRANCH}:refs/remotes/origin/${SHARED_BASELINE_BOT_BRANCH}`,
		]);
		assert.equal(
			requireLocalGit(fixture.runnerDirectory, ["rev-parse", `origin/${SHARED_BASELINE_BOT_BRANCH}^`]),
			fixture.mainSha,
		);
		assert.deepEqual(
			requireLocalGit(fixture.runnerDirectory, [
				"diff",
				"--name-only",
				fixture.mainSha,
				`origin/${SHARED_BASELINE_BOT_BRANCH}`,
			])
				.split(/\r?\n/)
				.filter(Boolean),
			[LOCAL_GIT_SNAPSHOT_PATH],
		);
	} finally {
		fixture.cleanup();
	}
	assert.equal(existsSync(fixture.temporaryDirectory), false);
	assert.deepEqual(captureLocalRepositoryState(repositoryRoot), developerState);
});

test("TASK-002 local-real Git force-with-lease preserves a concurrent bot update", () => {
	const fixture = createLocalGitFixture({ botBase: "stale" });
	let concurrentSha = "";
	try {
		const edge = createLocalGitCommandEdge({
			cwd: fixture.runnerDirectory,
			botBranchSha: fixture.botBranchSha,
			changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
			failWhen: (call) => {
				if (call.command !== "git" || call.args[0] !== "push" || concurrentSha) return undefined;
				requireLocalGit(fixture.sourceDirectory, ["checkout", "-B", "concurrent-bot", fixture.botBranchSha]);
				writeLocalGitFile(fixture.sourceDirectory, LOCAL_GIT_SNAPSHOT_PATH, "concurrent snapshot\n");
				requireLocalGit(fixture.sourceDirectory, ["add", "--", LOCAL_GIT_SNAPSHOT_PATH]);
				requireLocalGit(fixture.sourceDirectory, ["commit", "-m", "concurrent bot update"]);
				concurrentSha = requireLocalGit(fixture.sourceDirectory, ["rev-parse", "HEAD"]);
				requireLocalGit(fixture.sourceDirectory, ["push", "--force", "origin", `HEAD:${SHARED_BASELINE_BOT_BRANCH}`]);
				return undefined;
			},
		});

		assert.throws(
			() => reconcileBotCandidate({
				repository: REPOSITORY,
				mainSha: fixture.mainSha,
				botBranchExists: true,
				botBranchSha: fixture.botBranchSha,
				changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
				sourceRunUrl: SOURCE_RUN_URL,
				commandRunner: edge.run,
			}),
			/git command failed with exit code 1/,
		);
		assert.notEqual(concurrentSha, "");
		assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && ["create", "edit", "merge"].includes(call.args[1])).length, 0);
		requireLocalGit(fixture.runnerDirectory, [
			"fetch",
			"--force",
			"origin",
			`refs/heads/${SHARED_BASELINE_BOT_BRANCH}:refs/remotes/origin/${SHARED_BASELINE_BOT_BRANCH}`,
		]);
		assert.equal(
			requireLocalGit(fixture.runnerDirectory, ["rev-parse", `origin/${SHARED_BASELINE_BOT_BRANCH}`]),
			concurrentSha,
		);
	} finally {
		fixture.cleanup();
	}
});

test("TASK-001 local-real Git preserves the current-main exact-candidate characterization", () => {
	const developerState = captureLocalRepositoryState(repositoryRoot);
	const fixture = createLocalGitFixture({ botBase: "current" });
	try {
		assert.equal(
			requireLocalGit(fixture.runnerDirectory, ["rev-parse", `origin/${SHARED_BASELINE_BOT_BRANCH}^`]),
			fixture.mainSha,
		);
		const remotePaths = requireLocalGit(fixture.runnerDirectory, [
			"diff",
			"--name-only",
			fixture.mainSha,
			`origin/${SHARED_BASELINE_BOT_BRANCH}`,
		])
			.split(/\r?\n/)
			.filter(Boolean);
		assert.deepEqual(remotePaths, [LOCAL_GIT_SNAPSHOT_PATH]);

		const edge = createLocalGitCommandEdge({
			cwd: fixture.runnerDirectory,
			botBranchSha: fixture.botBranchSha,
			changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
			pullRequestLists: [[], []],
		});
		const result = reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: fixture.mainSha,
			botBranchExists: true,
			botBranchSha: fixture.botBranchSha,
			changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
			sourceRunUrl: SOURCE_RUN_URL,
			commandRunner: edge.run,
		});

		assert.equal(result.action, "merge-requested");
		assert.deepEqual(result.remotePaths, [LOCAL_GIT_SNAPSHOT_PATH]);
		assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "commit").length, 0);
		assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 0);
		const mergeCall = edge.calls.find((call) => call.command === "gh" && call.args[1] === "merge");
		assert.deepEqual(mergeCall.args.slice(-2), ["--match-head-commit", fixture.botBranchSha]);
	} finally {
		fixture.cleanup();
	}
	assert.equal(existsSync(fixture.temporaryDirectory), false);
	assert.deepEqual(captureLocalRepositoryState(repositoryRoot), developerState);
});

test("TASK-001 cleans fixture setup and command failures without changing repository refs", () => {
	const developerState = captureLocalRepositoryState(repositoryRoot);
	let failedSetupDirectory = "";
	assert.throws(
		() =>
			createLocalGitFixture({
				botBase: "invalid",
				onTemporaryDirectory: (directory) => {
					failedSetupDirectory = directory;
				},
			}),
		/unsupported local Git fixture bot base/,
	);
	assert.equal(existsSync(failedSetupDirectory), false);

	const fixture = createLocalGitFixture({ botBase: "stale" });
	const fixtureState = captureLocalRepositoryState(fixture.runnerDirectory);
	try {
		const edge = createLocalGitCommandEdge({
			cwd: fixture.runnerDirectory,
			botBranchSha: fixture.botBranchSha,
			failWhen: (call) => call.command === "git" && call.args[0] === "diff" ? 23 : undefined,
		});
		assert.throws(
			() => reconcileBotCandidate({
				repository: REPOSITORY,
				mainSha: fixture.mainSha,
				botBranchExists: true,
				botBranchSha: fixture.botBranchSha,
				changedPaths: [LOCAL_GIT_SNAPSHOT_PATH],
				sourceRunUrl: SOURCE_RUN_URL,
				commandRunner: edge.run,
			}),
			/git command failed with exit code 23/,
		);
		assert.equal(callsMatching(edge.calls, (call) => call.command === "gh").length, 0);
		assert.deepEqual(captureLocalRepositoryState(fixture.runnerDirectory), fixtureState);
	} finally {
		fixture.cleanup();
	}
	assert.equal(existsSync(fixture.temporaryDirectory), false);
	assert.deepEqual(captureLocalRepositoryState(repositoryRoot), developerState);
});

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
	assert.doesNotMatch(workflow, /backend-unit-latency-evidence/);
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
	assert.match(makefile, /-mode regenerate -skip-unit-latency/);
	assert.match(workflow, /node scripts\/ci\/shared-baseline-regeneration-workflow\.mjs reconcile/);
	assert.match(workflow, /CHANGED_PATHS: \$\{\{ steps\.candidate\.outputs\.paths \}\}/);
	assert.match(
		workflow,
		/validate-working-tree[\s\S]+--source-sha "\$SOURCE_SHA"[\s\S]+--github-output "\$GITHUB_OUTPUT"/,
	);
	assert.match(helperSource, /quiescent/);
	assert.match(helperSource, /reference\.baseCommit/);
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
	const checkoutStart = workflow.indexOf("      - name: Check out the delivered main revision");
	const checkoutEnd = workflow.indexOf("\n      - name: ", checkoutStart + 1);
	const checkoutStep = workflow.slice(checkoutStart, checkoutEnd === -1 ? workflow.length : checkoutEnd);
	assert.match(checkoutStep, /token: \$\{\{ steps\.bot-token\.outputs\.token \}\}/);
	assert.match(checkoutStep, /persist-credentials: false/);
	assert.doesNotMatch(checkoutStep, /GITHUB_TOKEN|github\.token|secrets\.SHARED_BASELINE_BOT_TOKEN/);
	assert.ok(
		checkoutStep.indexOf("token: ${{ steps.bot-token.outputs.token }}") <
			checkoutStep.indexOf("persist-credentials: false"),
		"checkout must disable persistence for the explicitly selected App credential",
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

test("F-06/F-20 local-real validation quiesces an identity-only unit budget and restores the clean tree", () => {
	const fixture = createWorkingTreeValidationFixture({ sourcePath: LOCAL_GIT_SNAPSHOT_PATH });
	try {
		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.match(
			result.stdout,
			/SHARED_BASELINE_CHANGED=false paths=\(none\) quiescent=true reason=.*reference\.baseCommit/,
		);
		assert.equal(readFileSync(fixture.githubOutput, "utf8").split(/\r?\n/).filter(Boolean)[0], "changed=false");
		assert.match(readFileSync(fixture.githubOutput, "utf8"), /paths=\nquiescent=true\n/);
		assert.equal(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		assert.equal(
			readFileSync(join(fixture.repository, SHARED_BASELINE_UNIT_BUDGET_PATH), "utf8"),
			fixture.currentBudgetContents,
		);
	} finally {
		fixture.cleanup();
	}
});

test("F-07 retains material unit-budget JSON drift for normal publication", () => {
	const fixture = createWorkingTreeValidationFixture({ candidate: "material" });
	try {
		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.match(
			result.stdout,
			new RegExp(`SHARED_BASELINE_CHANGED=true paths=${SHARED_BASELINE_UNIT_BUDGET_PATH.replaceAll("/", "\\/")} quiescent=false`),
		);
		assert.match(readFileSync(fixture.githubOutput, "utf8"), /changed=true/);
		assert.match(readFileSync(fixture.githubOutput, "utf8"), /quiescent=false/);
		assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
	} finally {
		fixture.cleanup();
	}
});

test("F-08 does not quiesce a baseCommit-only candidate when the source commit has real non-baseline drift", () => {
	const fixture = createWorkingTreeValidationFixture({ sourcePath: "src/real-source-change.go" });
	try {
		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.match(result.stdout, /SHARED_BASELINE_CHANGED=true/);
		assert.match(result.stdout, /quiescent=false/);
		assert.match(result.stdout, /src\/real-source-change\.go/);
		assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
	} finally {
		fixture.cleanup();
	}
});

test("F-01 accepts only a deterministic two-parent first-parent source merge", () => {
	const fixture = createWorkingTreeValidationFixture({ sourceTopology: "two-parent-merge" });
	try {
		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.match(result.stdout, /SHARED_BASELINE_CHANGED=false.*quiescent=true/);
		assert.equal(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
	} finally {
		fixture.cleanup();
	}
});

test("F-01 validates an oversized identity-only budget through the real helper CLI", () => {
	const fixture = createWorkingTreeValidationFixture({
		sourceTopology: "two-parent-merge",
		oversized: true,
	});
	try {
		const budgetContents = readFileSync(
			join(fixture.repository, SHARED_BASELINE_UNIT_BUDGET_PATH),
			"utf8",
		);
		assert.ok(
			Buffer.byteLength(budgetContents, "utf8") > NODE_DEFAULT_CHILD_OUTPUT_BUFFER_BYTES,
			`oversized fixture must exceed Node's ${NODE_DEFAULT_CHILD_OUTPUT_BUFFER_BYTES}-byte child-output buffer; received ${Buffer.byteLength(budgetContents, "utf8")} bytes`,
		);

		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.doesNotMatch(`${result.stdout}${result.stderr}`, /ENOBUFS/);
		assert.match(
			result.stdout,
			/SHARED_BASELINE_CHANGED=false paths=\(none\) quiescent=true reason=.*reference\.baseCommit/,
		);
		assert.equal(readFileSync(fixture.githubOutput, "utf8"), "changed=false\npaths=\nquiescent=true\nreason=source revision changed only allowlisted snapshots and the unit-latency budget differs only at reference.baseCommit; restored the identity-only candidate\n");
		assert.equal(
			readFileSync(join(fixture.repository, SHARED_BASELINE_UNIT_BUDGET_PATH), "utf8"),
			fixture.currentBudgetContents,
		);
		assert.equal(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		assert.deepEqual(readdirSync(fixture.blobTemporaryDirectory), []);
	} finally {
		fixture.cleanup();
	}
});

test("F-02 retains oversized material budget drift for normal publication", () => {
	const fixture = createWorkingTreeValidationFixture({
		sourceTopology: "two-parent-merge",
		candidate: "material",
		oversized: true,
	});
	try {
		const budgetContents = readFileSync(
			join(fixture.repository, SHARED_BASELINE_UNIT_BUDGET_PATH),
			"utf8",
		);
		assert.ok(Buffer.byteLength(budgetContents, "utf8") > NODE_DEFAULT_CHILD_OUTPUT_BUFFER_BYTES);
		const result = runWorkingTreeValidation(fixture);
		assert.equal(result.status, 0, result.stderr);
		assert.doesNotMatch(`${result.stdout}${result.stderr}`, /ENOBUFS/);
		assert.match(
			result.stdout,
			new RegExp(
				`SHARED_BASELINE_CHANGED=true paths=${SHARED_BASELINE_UNIT_BUDGET_PATH.replaceAll("/", "\\/")} quiescent=false reason=.*material JSON drift`,
			),
		);
		assert.match(readFileSync(fixture.githubOutput, "utf8"), /changed=true/);
		assert.match(readFileSync(fixture.githubOutput, "utf8"), /quiescent=false/);
		assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		assert.deepEqual(readdirSync(fixture.blobTemporaryDirectory), []);
	} finally {
		fixture.cleanup();
	}
});

test("F-03 rejects unexpected and untracked candidate paths before restore or output", () => {
	for (const { path, add, message } of [
		{
			path: "docs/internal/baselines/unexpected.txt",
			add: true,
			message: /unexpected path\(s\).*unexpected\.txt/,
		},
		{
			path: "untracked-candidate.txt",
			add: false,
			message: /untracked path\(s\).*untracked-candidate\.txt/,
		},
	]) {
		const fixture = createWorkingTreeValidationFixture();
		try {
			writeLocalGitFile(fixture.repository, path, "invalid candidate\n");
			if (add) requireLocalGit(fixture.repository, ["add", "--", path]);
			const result = runWorkingTreeValidation(fixture);
			assert.notEqual(result.status, 0);
			assert.match(result.stderr, message);
			assert.equal(existsSync(fixture.githubOutput), false);
			assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		} finally {
			fixture.cleanup();
		}
	}
});

test("F-04 rejects source renames and copies involving the allowlist", () => {
	for (const sourceOperation of ["rename", "copy"]) {
		const fixture = createWorkingTreeValidationFixture({ sourceOperation });
		try {
			const result = runWorkingTreeValidation(fixture);
			assert.notEqual(result.status, 0, `${sourceOperation}: ${result.stdout}${result.stderr}`);
			assert.match(result.stderr, /rename or copy path record/);
			assert.equal(existsSync(fixture.githubOutput), false);
			assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		} finally {
			fixture.cleanup();
		}
	}
});

test("F-05 rejects a source revision that is not the checked-out HEAD before mutation", () => {
	const fixture = createWorkingTreeValidationFixture();
	try {
		const wrongSource = requireLocalGit(fixture.repository, ["rev-parse", `${fixture.sourceSha}^`]);
		const result = runWorkingTreeValidation(fixture, [], { sourceSha: wrongSource });
		assert.notEqual(result.status, 0);
		assert.match(result.stderr, /checked-out HEAD .* does not match source revision/);
		assert.equal(existsSync(fixture.githubOutput), false);
	} finally {
		fixture.cleanup();
	}
});

test("F-07 keeps malformed budget candidates dirty and writes no classification output", () => {
	for (const contents of ["{ malformed\n", '{"reference":[]}', '{"reference":{"baseCommit":"ABC"}}']) {
		const fixture = createWorkingTreeValidationFixture();
		try {
			writeLocalGitFile(fixture.repository, SHARED_BASELINE_UNIT_BUDGET_PATH, contents);
			const result = runWorkingTreeValidation(fixture);
			assert.notEqual(result.status, 0);
			assert.match(result.stderr, /generated unit-latency budget/);
			assert.equal(existsSync(fixture.githubOutput), false);
			assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		} finally {
			fixture.cleanup();
		}
	}
});

test("F-08 cleans the blob-transfer temporary root after a failed run and a clean retry", () => {
	const fixture = createWorkingTreeValidationFixture({ oversized: true });
	const invalidTemporaryRoot = join(fixture.temporaryDirectory, "not-a-directory");
	try {
		writeFileSync(invalidTemporaryRoot, "not a directory\n", "utf8");
		const failed = runWorkingTreeValidation(fixture, [], {
			env: {
				TMPDIR: invalidTemporaryRoot,
				TMP: invalidTemporaryRoot,
				TEMP: invalidTemporaryRoot,
			},
		});
		assert.notEqual(failed.status, 0);
		assert.doesNotMatch(`${failed.stdout}${failed.stderr}`, /testInventory|TestOversizedBudget/);
		assert.equal(existsSync(fixture.githubOutput), false);
		assert.notEqual(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");

		const retry = runWorkingTreeValidation(fixture);
		assert.equal(retry.status, 0, retry.stderr);
		assert.match(retry.stdout, /SHARED_BASELINE_CHANGED=false.*quiescent=true/);
		assert.equal(requireLocalGit(fixture.repository, ["status", "--porcelain"]), "");
		assert.deepEqual(readdirSync(fixture.blobTemporaryDirectory), []);
	} finally {
		fixture.cleanup();
	}
});

test("F-12 retains sorted material paths for empty, single-snapshot, and multi-snapshot candidates", () => {
	const emptyFixture = createWorkingTreeValidationFixture();
	try {
		requireLocalGit(emptyFixture.repository, ["restore", "--source=HEAD", "--staged", "--worktree", "--", SHARED_BASELINE_UNIT_BUDGET_PATH]);
		const result = runWorkingTreeValidation(emptyFixture);
		assert.equal(result.status, 0, result.stderr);
		assert.match(result.stdout, /SHARED_BASELINE_CHANGED=false paths=\(none\) quiescent=false reason=no generated shared baseline changes/);
	} finally {
		emptyFixture.cleanup();
	}

	const multiFixture = createWorkingTreeValidationFixture();
	try {
		writeLocalGitFile(multiFixture.repository, LOCAL_GIT_SNAPSHOT_PATH, "another generated snapshot\n");
		const result = runWorkingTreeValidation(multiFixture);
		assert.equal(result.status, 0, result.stderr);
		const expectedPaths = [SHARED_BASELINE_UNIT_BUDGET_PATH, LOCAL_GIT_SNAPSHOT_PATH].sort().join(",");
		assert.match(result.stdout, new RegExp(`SHARED_BASELINE_CHANGED=true paths=${expectedPaths.replaceAll("/", "\\/")} quiescent=false`));
		assert.notEqual(requireLocalGit(multiFixture.repository, ["status", "--porcelain"]), "");
	} finally {
		multiFixture.cleanup();
	}
});

test("F-01/F-19 fail closed for invalid source and malformed budget evidence before mutation", () => {
	const invalidSourceFixture = createWorkingTreeValidationFixture();
	try {
		const result = spawnSync(
			process.execPath,
			[helper, "validate-working-tree", "--source-sha", "not-a-commit", "--github-output", invalidSourceFixture.githubOutput],
			{
				cwd: invalidSourceFixture.repository,
				env: { ...process.env, GIT_CONFIG_NOSYSTEM: "1" },
				encoding: "utf8",
				windowsHide: true,
			},
		);
		assert.notEqual(result.status, 0);
		assert.match(result.stderr, /source revision must be a complete commit SHA/);
		assert.equal(existsSync(invalidSourceFixture.githubOutput), false);
	} finally {
		invalidSourceFixture.cleanup();
	}

	const malformedFixture = createWorkingTreeValidationFixture({ candidate: "malformed" });
	try {
		const result = runWorkingTreeValidation(malformedFixture);
		assert.notEqual(result.status, 0);
		assert.match(result.stderr, /generated unit-latency budget is not valid JSON/);
		assert.equal(existsSync(malformedFixture.githubOutput), false);
		assert.notEqual(requireLocalGit(malformedFixture.repository, ["status", "--porcelain"]), "");
	} finally {
		malformedFixture.cleanup();
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

test("AUTH-04/F-04 no-diff reconciliation is a no-op without branch or pull-request mutation", () => {
	const edge = createControlledCommandEdge({ pullRequestLists: [[]] });
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [],
		commandRunner: edge.run,
	});

	assert.deepEqual(result, { action: "noop", publish: false, remotePaths: [] });
	assert.equal(callsMatching(edge.calls, (call) => call.command === "gh" && call.args[0] === "pr" && call.args[1] === "list").length, 1);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 0);
	assert.equal(
		callsMatching(
			edge.calls,
			(call) => call.command === "gh" && ["close", "create", "edit", "ready", "merge"].includes(call.args[1]),
		).length,
		0,
	);
});

test("AUTH-05 rejected App authentication fails before push or later pull-request mutation", () => {
	const edge = createControlledCommandEdge({
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		failWhen: (call) =>
			call.command === "gh" && call.args[0] === "auth" && call.args[1] === "setup-git" ? 29 : undefined,
	});
	assert.throws(
		() =>
			reconcileBotCandidate({
				repository: REPOSITORY,
				mainSha: MAIN_SHA,
				changedPaths: [SHARED_BASELINE_PATHS[0]],
				sourceRunUrl: SOURCE_RUN_URL,
				commandRunner: edge.run,
			}),
		/gh command failed with exit code 29/,
	);

	const setupCalls = edge.calls.filter(
		(call) => call.command === "gh" && call.args[0] === "auth" && call.args[1] === "setup-git",
	);
	assert.equal(setupCalls.length, 1);
	assert.equal(callsMatching(edge.calls, (call) => call.command === "git" && call.args[0] === "push").length, 0);
	assert.equal(
		callsMatching(
			edge.calls,
			(call) => call.command === "gh" && ["close", "create", "edit", "ready", "merge"].includes(call.args[1]),
		).length,
		0,
	);
	assert.doesNotMatch(edge.calls.map(commandText).join("\n"), /default-fixture-token|app-fixture-token/);
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
	const duplicatedPaths = SHARED_BASELINE_PATHS
		.slice()
		.reverse()
		.flatMap((path) => [path, path]);
	assert.deepEqual(validateAllowlistedPaths(duplicatedPaths), SHARED_BASELINE_PATHS.slice().sort());
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

	const remoteEdge = createControlledCommandEdge({ remotePaths: [invalidPath], stagedPaths: [] });
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

test("F-12 rejects duplicate bot PRs before branch publication", () => {
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

test("F-14 makes an exact draft PR ready before requesting exact-head auto-merge", () => {
	const changedPath = SHARED_BASELINE_PATHS[0];
	const edge = createControlledCommandEdge({
		pullRequestLists: [[], []],
		stagedPaths: [changedPath],
		metadata: { files: [changedPath], isDraft: true },
	});
	const result = reconcileBotCandidate({
		repository: REPOSITORY,
		mainSha: MAIN_SHA,
		changedPaths: [changedPath],
		sourceRunUrl: SOURCE_RUN_URL,
		commandRunner: edge.run,
	});

	assert.equal(result.action, "merge-requested");
	const readyIndex = edge.calls.findIndex((call) => call.command === "gh" && call.args[1] === "ready");
	const mergeIndex = edge.calls.findIndex((call) => call.command === "gh" && call.args[1] === "merge");
	assert.ok(readyIndex >= 0);
	assert.ok(mergeIndex > readyIndex);
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

	const mismatchedFilesEdge = createControlledCommandEdge({
		pullRequestLists: [[], []],
		stagedPaths: [SHARED_BASELINE_PATHS[0]],
		metadata: { files: [SHARED_BASELINE_PATHS[0], SHARED_BASELINE_PATHS[1]] },
	});
	assert.throws(
		() => reconcileBotCandidate({
			repository: REPOSITORY,
			mainSha: MAIN_SHA,
			changedPaths: [SHARED_BASELINE_PATHS[0]],
			sourceRunUrl: "https://github.example/actions/runs/42",
			commandRunner: mismatchedFilesEdge.run,
		}),
		/pull request files do not match generated candidate/,
	);
	assert.equal(callsMatching(mismatchedFilesEdge.calls, (call) => call.command === "gh" && ["ready", "merge"].includes(call.args[1])).length, 0);

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
