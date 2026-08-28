import { appendFileSync, readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const SHARED_BASELINE_PATHS = Object.freeze([
	"docs/internal/baselines/deadcode-baseline.txt",
	"docs/internal/baselines/go-unit-lane-latency-budget.v1.json",
]);

export const SHARED_BASELINE_BOT_BRANCH = "automation/shared-ci-baselines";
export const SHARED_BASELINE_PR_TITLE = "chore(ci): reconcile shared CI baselines";
export const SHARED_BASELINE_COMMENT_MARKER =
	"<!-- shared-ci-baseline-regeneration -->";

const COMMIT_SHA_PATTERN = /^[0-9a-f]{40}$/i;

function normalizePath(path) {
	return String(path || "")
		.replaceAll("\\", "/")
		.replace(/^\.\//, "")
		.trim();
}

function sortedUnique(paths) {
	return [...new Set(paths.map(normalizePath).filter(Boolean))].sort((left, right) =>
		left < right ? -1 : left > right ? 1 : 0,
	);
}

export function parsePorcelainPaths(status) {
	const paths = [];
	for (const line of String(status || "").split(/\r?\n/)) {
		if (line.length < 3) continue;
		const path = line.slice(3);
		if (path.includes(" -> ")) {
			paths.push(...path.split(" -> "));
		} else {
			paths.push(path);
		}
	}
	return sortedUnique(paths);
}

export function validateAllowlistedPaths(paths, { requireChanges = false } = {}) {
	const normalized = sortedUnique(paths);
	const allowed = new Set(SHARED_BASELINE_PATHS);
	const unexpected = normalized.filter((path) => !allowed.has(path));
	if (unexpected.length > 0) {
		throw new Error(
			`shared baseline reconciliation may modify only ${SHARED_BASELINE_PATHS.join(", ")}; unexpected path(s): ${unexpected.join(", ")}`,
		);
	}
	if (requireChanges && normalized.length === 0) {
		throw new Error("shared baseline reconciliation expected a generated change, but the working tree is clean");
	}
	return normalized;
}

export function selectSourceWorkflowRun({
	workflowName = "",
	headBranch = "",
	conclusion = "",
	defaultBranch = "main",
} = {}) {
	if (workflowName !== "CI") {
		return { selected: false, reason: `workflow ${workflowName || "(unknown)"} is not CI` };
	}
	if (headBranch !== defaultBranch) {
		return {
			selected: false,
			reason: `workflow head branch ${headBranch || "(unknown)"} is not ${defaultBranch}`,
		};
	}
	if (["cancelled", "timed_out", "action_required", "stale"].includes(conclusion)) {
		return {
			selected: false,
			reason: `source CI conclusion is ${conclusion || "(unknown)"}; a completed run with unavailable evidence cannot be used`,
		};
	}
	return {
		selected: conclusion === "success" || conclusion === "failure",
		reason:
			conclusion === "failure"
				? "completed CI failure may reflect stale baselines; the hosted artifact and generator remain the publication gates"
				: "completed successful CI run on the default branch",
	};
}

export function planReconciliation({
	triggeringSha = "",
	currentMainSha = "",
	changedPaths = [],
	candidateMatchesRemote = false,
	existingPullRequest = false,
	generationError = "",
} = {}) {
	if (generationError) {
		return {
			action: "fail",
			publish: false,
			reason: `generation failed before publication: ${generationError}`,
		};
	}
	if (!COMMIT_SHA_PATTERN.test(triggeringSha) || !COMMIT_SHA_PATTERN.test(currentMainSha)) {
		return {
			action: "fail",
			publish: false,
			reason: "the triggering and current main revisions must both be complete commit SHAs",
		};
	}
	if (triggeringSha !== currentMainSha) {
		return {
			action: "superseded",
			publish: false,
			reason: `main moved from ${triggeringSha} to ${currentMainSha}; a newer run owns reconciliation`,
		};
	}
	const paths = validateAllowlistedPaths(changedPaths);
	if (paths.length === 0) {
		return {
			action: existingPullRequest ? "close-existing" : "noop",
			publish: false,
			reason: existingPullRequest
				? "main already contains both generated baselines; the open automation PR is obsolete"
				: "main already contains both generated baselines",
		};
	}
	if (candidateMatchesRemote) {
		return {
			action: existingPullRequest ? "reuse-pr" : "reuse-branch",
			publish: false,
			reason: "the existing automation branch already contains this exact candidate",
		};
	}
	return {
		action: "publish",
		publish: true,
		reason: `publish generated change(s) in ${paths.join(", ")}`,
	};
}

export function renderPullRequestBody({
	sourceSha,
	commitSha,
	runUrl,
	changedPaths = SHARED_BASELINE_PATHS,
} = {}) {
	const paths = validateAllowlistedPaths(changedPaths, { requireChanges: true });
	return [
		SHARED_BASELINE_COMMENT_MARKER,
		"## Automated shared CI baseline reconciliation",
		"",
		"This pull request was generated from the completed main-branch CI run below. Its hosted unit-latency artifact and the current deadcode report passed the generator gates. It is intentionally limited to the two self-maintaining shared baselines:",
		"",
		...paths.map((path) => "- " + "\x60" + path + "\x60"),
		"",
		`- Source main revision: \`${sourceSha}\``,
		`- Generated branch revision: \`${commitSha}\``,
		`- Source CI run: ${runUrl}`,
		`- Automation branch: \`${SHARED_BASELINE_BOT_BRANCH}\``,
		`- Pull request title: \`${SHARED_BASELINE_PR_TITLE}\``,
		"",
		"The branch is reconciled from the latest main revision on each run. Required repository checks own the merge gate; auto-merge is enabled after the bot verifies the PR head and file allowlist.",
		"",
		"<!-- This content is generated. Do not edit this pull request manually. -->",
		"",
	].join("\n");
}

function runGit(args) {
	const result = spawnSync("git", args, { encoding: "utf8", windowsHide: true });
	if (result.error) throw new Error(`unable to execute git: ${result.error.message}`);
	if (result.status !== 0) {
		const detail = `${result.stderr || result.stdout || ""}`.trim();
		throw new Error(`git ${args.join(" ")} failed with exit code ${result.status}${detail ? `: ${detail}` : ""}`);
	}
	return result.stdout || "";
}

function writeGitHubOutput(path, values) {
	if (!path) return;
	for (const [key, value] of Object.entries(values)) {
		appendFileSync(path, `${key}=${value}\n`);
	}
}

function runValidateWorkingTree({ staged = false, requireChanges = false, githubOutput = "" } = {}) {
	const status = staged
		? runGit(["diff", "--cached", "--name-only"])
		: runGit(["status", "--porcelain=v1", "--untracked-files=all"]);
	const paths = staged ? sortedUnique(status.split(/\r?\n/)) : parsePorcelainPaths(status);
	validateAllowlistedPaths(paths, { requireChanges });
	writeGitHubOutput(githubOutput, {
		changed: paths.length > 0 ? "true" : "false",
		paths: paths.join(","),
	});
	process.stdout.write(
		`SHARED_BASELINE_CHANGED=${paths.length > 0 ? "true" : "false"} paths=${paths.join(",") || "(none)"}\n`,
	);
}

function parseArguments(args) {
	const [command, ...rest] = args;
	const options = { command };
	for (let index = 0; index < rest.length; index += 1) {
		const argument = rest[index];
		if (argument === "--github-output") {
			options.githubOutput = rest[++index];
			if (!options.githubOutput) throw new Error("--github-output requires a path");
			continue;
		}
		if (argument === "--require-changes") {
			options.requireChanges = true;
			continue;
		}
		if (argument === "--staged") {
			options.staged = true;
			continue;
		}
		if (argument === "--source-sha" || argument === "--commit-sha" || argument === "--run-url") {
			options[argument.slice(2).replaceAll("-", "")] = rest[++index];
			if (!options[argument.slice(2).replaceAll("-", "")]) throw new Error(`${argument} requires a value`);
			continue;
		}
		if (argument === "--paths") {
			options.paths = rest[++index]?.split(",") || [];
			continue;
		}
		throw new Error(`Unknown argument: ${argument}`);
	}
	return options;
}

function runCli(args) {
	const options = parseArguments(args);
	if (options.command === "validate-working-tree") {
		runValidateWorkingTree(options);
		return;
	}
	if (options.command === "validate-staged") {
		runValidateWorkingTree({ ...options, staged: true, requireChanges: true });
		return;
	}
	if (options.command === "validate-paths") {
		const paths = readFileSync(0, "utf8").split(/\r?\n/).filter(Boolean);
		validateAllowlistedPaths(paths);
		process.stdout.write(`SHARED_BASELINE_PATHS_OK paths=${sortedUnique(paths).join(",") || "(none)"}\n`);
		return;
	}
	if (options.command === "render-body") {
		process.stdout.write(
			renderPullRequestBody({
				sourceSha: options.sourcesha,
				commitSha: options.commitsha,
				runUrl: options.runurl,
				changedPaths: options.paths?.length ? options.paths : SHARED_BASELINE_PATHS,
			}),
		);
		return;
	}
	throw new Error(`Unknown command: ${options.command || "(missing)"}`);
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try {
		runCli(process.argv.slice(2));
	} catch (error) {
		process.stderr.write(`shared baseline workflow: ${error.message}\n`);
		process.exitCode = 1;
	}
}
