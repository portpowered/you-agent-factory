import { appendFileSync, closeSync, mkdtempSync, openSync, readFileSync, rmSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";

export const SHARED_BASELINE_PATHS = Object.freeze([
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

export const SHARED_BASELINE_UNIT_BUDGET_PATH =
	"docs/internal/baselines/go-unit-lane-latency-budget.v1.json";

export const SHARED_BASELINE_BOT_BRANCH = "automation/shared-ci-baselines";
export const SHARED_BASELINE_PR_TITLE = "chore(ci): reconcile shared CI baselines";
export const SHARED_BASELINE_COMMENT_MARKER =
	"<!-- shared-ci-baseline-regeneration -->";

const COMMIT_SHA_PATTERN = /^[0-9a-f]{40}$/i;
const LOWERCASE_COMMIT_SHA_PATTERN = /^[0-9a-f]{40}$/;
const SHARED_BASELINE_PATH_SET = new Set(SHARED_BASELINE_PATHS);
const MAX_METADATA_OUTPUT_BYTES = 64 * 1024;
const NO_DIFF_PULL_REQUEST_COMMENT =
	"The latest successful main CI run regenerated no changes; this reconciliation is no longer needed.";

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

function parseGitLinePaths(output, label) {
	const rawPaths = String(output || "")
		.split(/\r?\n/)
		.filter(Boolean)
		.map(normalizePath);
	const paths = sortedUnique(rawPaths);
	if (paths.length !== rawPaths.length) {
		throw new Error(`${label} contained duplicate or ambiguous path entries`);
	}
	return paths;
}

function parseGitNameStatusPaths(output, label) {
	const fields = String(output || "").split("\0").filter(Boolean);
	const rawPaths = [];
	for (let index = 0; index < fields.length; ) {
		const status = fields[index++];
		if (status.startsWith("R") || status.startsWith("C")) {
			throw new Error(`${label} contained a rename or copy path record`);
		}
		if (!/^[A-Z]$/.test(status) || index >= fields.length || !fields[index]) {
			throw new Error(`${label} contained an invalid path record`);
		}
		rawPaths.push(normalizePath(fields[index++]));
	}
	const paths = sortedUnique(rawPaths.filter(Boolean));
	if (paths.length !== rawPaths.filter(Boolean).length) {
		throw new Error(`${label} contained duplicate or ambiguous path entries`);
	}
	return paths;
}

function parseCandidatePorcelainPaths(status) {
	const rawPaths = [];
	for (const line of String(status || "").split(/\r?\n/)) {
		if (line.length < 3) continue;
		const indexStatus = line[0];
		const worktreeStatus = line[1];
		const path = line.slice(3);
		if (indexStatus === "?" || worktreeStatus === "?") {
			throw new Error(`generated candidate contains untracked path(s): ${normalizePath(path)}`);
		}
		if (
			indexStatus === "R" ||
			indexStatus === "C" ||
			worktreeStatus === "R" ||
			worktreeStatus === "C" ||
			path.includes(" -> ")
		) {
			throw new Error("generated candidate contains a rename or copy path record");
		}
		rawPaths.push(normalizePath(path));
	}
	const normalizedPaths = rawPaths.filter(Boolean);
	const paths = sortedUnique(normalizedPaths);
	if (paths.length !== normalizedPaths.length) {
		throw new Error("generated candidate contained duplicate or ambiguous path entries");
	}
	return paths;
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
	if (conclusion !== "success" && conclusion !== "failure") {
		return {
			selected: false,
			reason: `source CI conclusion is ${conclusion || "(unknown)"}; only success or failure runs may reach checkout, generation, or publication`,
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
				? "main already contains all classified shared baselines; the open automation PR is obsolete"
				: "main already contains all classified shared baselines",
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
		"This pull request was generated from the completed main-branch CI run below. Its hosted unit-latency artifact and the current deadcode report passed the generator gates. It is intentionally limited to the classified self-maintaining shared baseline snapshots:",
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

function requireCommitSha(value, label) {
	if (!COMMIT_SHA_PATTERN.test(String(value || ""))) {
		throw new Error(`${label} must be a complete commit SHA`);
	}
}

function parseJsonOutput(output, label) {
	try {
		return JSON.parse(output);
	} catch (error) {
		throw new Error(`${label} returned invalid JSON: ${error.message}`);
	}
}

function parsePullRequestNumber(value, label) {
	const number = Number(String(value || "").trim());
	if (!Number.isInteger(number) || number <= 0) {
		throw new Error(`${label} did not return a valid pull request number`);
	}
	return number;
}

function createCommandEdge(commandRunner = runExternalCommand) {
	function invoke(command, args, options = {}) {
		const result = normalizeCommandResult(commandRunner(command, args, options));
		if (!options.allowFailure && result.status !== 0) {
			const detail = result.stderr.trim();
			throw new Error(
				`${command} command failed with exit code ${result.status}${detail ? `: ${detail}` : ""}`,
			);
		}
		return result;
	}

	return {
		git(args, options) {
			return invoke("git", args, options);
		},
		gh(args, options) {
			return invoke("gh", args, options);
		},
	};
}

function readOpenPullRequests(edge, { repository, defaultBranch, botBranch }) {
	const output = edge.gh([
		"pr",
		"list",
		"--repo",
		repository,
		"--state",
		"open",
		"--base",
		defaultBranch,
		"--head",
		botBranch,
		"--json",
		"number,url",
		"--limit",
		"10",
	]).stdout;
	const pullRequests = parseJsonOutput(output, "gh pr list");
	if (!Array.isArray(pullRequests)) throw new Error("gh pr list returned a non-array result");
	return pullRequests;
}

function ensureSinglePullRequest(pullRequests, botBranch) {
	if (pullRequests.length > 1) {
		throw new Error(`found ${pullRequests.length} open pull requests for ${botBranch}; refusing to select one`);
	}
}

function checkCurrentMain(edge, { mainSha, defaultBranch, phase }) {
	edge.git([
		"fetch",
		"--force",
		"origin",
		`refs/heads/${defaultBranch}:refs/remotes/origin/${defaultBranch}`,
	], { maxBuffer: MAX_METADATA_OUTPUT_BYTES });
	const currentMainSha = edge.git(["rev-parse", `origin/${defaultBranch}`], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout.trim();
	if (currentMainSha === mainSha) return null;
	return {
		action: "superseded",
		publish: false,
		reason: `main moved from ${mainSha} to ${currentMainSha} before ${phase}; a newer run owns reconciliation`,
	};
}

function inspectRemoteBranch(edge, { mainSha, botBranch, expectedSha }) {
	const remoteRef = `origin/${botBranch}`;
	const actualSha = edge.git(["rev-parse", remoteRef], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout.trim();
	if (actualSha !== expectedSha) {
		throw new Error(
			`automation branch moved from ${expectedSha} to ${actualSha || "(unknown)"} before reconciliation; refusing to replace concurrent work`,
		);
	}
	const baseSha = edge.git(["merge-base", mainSha, remoteRef], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout.trim();
	requireCommitSha(baseSha, "automation branch base revision");
	const parentSha = edge.git(["rev-parse", `${remoteRef}^`], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout.trim();
	requireCommitSha(parentSha, "automation branch parent revision");
	const output = edge.git(["diff", "--name-only", baseSha, remoteRef], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout;
	return {
		baseSha,
		parentSha,
		paths: validateAllowlistedPaths(parseGitLinePaths(output, "automation branch diff")),
	};
}

function refreshRemoteBranch(edge, botBranch) {
	edge.git([
		"fetch",
		"--force",
		"origin",
		`refs/heads/${botBranch}:refs/remotes/origin/${botBranch}`,
	], { maxBuffer: MAX_METADATA_OUTPUT_BYTES });
}

function matchesRemoteCandidate(edge, botBranch) {
	let matches = true;
	for (const path of SHARED_BASELINE_PATHS) {
		const result = edge.git(
			["diff", "--quiet", `origin/${botBranch}`, "--", path],
			{ allowFailure: true },
		);
		if (result.status !== 0 && result.status !== 1) {
			throw new Error(`git candidate comparison failed with exit code ${result.status}`);
		}
		if (result.status === 1) matches = false;
	}
	return matches;
}

function validatePullRequestMetadata(metadata, {
	defaultBranch,
	botBranch,
	commitSha,
	candidatePaths,
}) {
	if (
		metadata.baseRefName !== defaultBranch ||
		metadata.headRefName !== botBranch
	) {
		throw new Error(
			`pull request does not target ${defaultBranch} from ${botBranch}`,
		);
	}
	if (metadata.headRefOid !== commitSha) {
		throw new Error(`pull request head does not match generated commit ${commitSha}`);
	}
	if (!Array.isArray(metadata.files)) throw new Error("pull request metadata did not include files");
	const pullRequestPaths = validateAllowlistedPaths(
		metadata.files.map((file) => file?.path),
		{ requireChanges: true },
	);
	if (pullRequestPaths.join("\n") !== candidatePaths.join("\n")) {
		throw new Error(
			`pull request files do not match generated candidate; expected ${candidatePaths.join(", ")}, received ${pullRequestPaths.join(", ")}`,
		);
	}
}

function verifyCandidateBase(edge, { mainSha, revision, label }) {
	const parentSha = edge.git(["rev-parse", `${revision}^`], {
		maxBuffer: MAX_METADATA_OUTPUT_BYTES,
	}).stdout.trim();
	if (parentSha !== mainSha) {
		throw new Error(
			`${label} parent ${parentSha || "(unknown)"} does not match guarded main ${mainSha}`,
		);
	}
}

function readGeneratedCandidatePaths(edge, { mainSha, changedPaths }) {
	const workingTreePaths = parseGitLinePaths(
		edge.git(["diff", "--name-only", mainSha], { maxBuffer: MAX_METADATA_OUTPUT_BYTES }).stdout,
		"generated working-tree diff",
	);
	const stagedPaths = parseGitLinePaths(
		edge.git(["diff", "--cached", "--name-only"], { maxBuffer: MAX_METADATA_OUTPUT_BYTES }).stdout,
		"generated staged diff",
	);
	return validateAllowlistedPaths([
		...changedPaths,
		...workingTreePaths,
		...stagedPaths,
	]);
}

export function reconcileBotCandidate({
	repository = "",
	defaultBranch = "main",
	botBranch = SHARED_BASELINE_BOT_BRANCH,
	prTitle = SHARED_BASELINE_PR_TITLE,
	mainSha = "",
	botBranchExists = false,
	botBranchSha = "",
	changedPaths = [],
	sourceRunUrl = "",
	commandRunner = runExternalCommand,
} = {}) {
	if (!String(repository).trim()) throw new Error("repository is required for bot reconciliation");
	requireCommitSha(mainSha, "main revision");
	if (botBranchExists) requireCommitSha(botBranchSha, "existing bot branch revision");
	const requestedCandidatePaths = validateAllowlistedPaths(changedPaths, {
		requireChanges: changedPaths.length > 0,
	});
	const edge = createCommandEdge(commandRunner);

	let reconciliation = checkCurrentMain(edge, {
		mainSha,
		defaultBranch,
		phase: "reconciliation",
	});
	if (reconciliation) return reconciliation;
	const candidatePaths = readGeneratedCandidatePaths(edge, {
		mainSha,
		changedPaths: requestedCandidatePaths,
	});
	if (candidatePaths.length > 0 && !String(sourceRunUrl).trim()) {
		throw new Error("source CI run URL is required for a generated pull request");
	}

	if (candidatePaths.length === 0) {
		let remoteBranch = null;
		if (botBranchExists) {
			refreshRemoteBranch(edge, botBranch);
			remoteBranch = inspectRemoteBranch(edge, {
				mainSha,
				botBranch,
				expectedSha: botBranchSha,
			});
		}
		let botBranchPresent = false;
		if (botBranchExists) {
			edge.gh(["auth", "setup-git", "--hostname", "github.com"]);
			const branch = edge.git(
				["ls-remote", "--exit-code", "--heads", "origin", botBranch],
				{ allowFailure: true },
			);
			if (branch.status !== 0 && branch.status !== 2) {
				throw new Error(`git bot branch lookup failed with exit code ${branch.status}`);
			}
			botBranchPresent = branch.status === 0;
		}
		const pullRequests = readOpenPullRequests(edge, { repository, defaultBranch, botBranch });
		ensureSinglePullRequest(pullRequests, botBranch);
		reconciliation = checkCurrentMain(edge, {
			mainSha,
			defaultBranch,
			phase: "no-diff cleanup",
		});
		if (reconciliation) return reconciliation;
		if (pullRequests.length === 1) {
			const pullRequestNumber = parsePullRequestNumber(
				pullRequests[0].number,
				"gh pr list",
			);
			edge.gh([
				"pr",
				"close",
				String(pullRequestNumber),
				"--repo",
				repository,
				"--comment",
				NO_DIFF_PULL_REQUEST_COMMENT,
			]);
		}
		if (botBranchPresent) {
			reconciliation = checkCurrentMain(edge, {
				mainSha,
				defaultBranch,
				phase: "obsolete branch cleanup",
			});
			if (reconciliation) return reconciliation;
			edge.git(["push", "origin", "--delete", botBranch]);
		}
		return {
			action: pullRequests.length === 1 ? "close-existing" : "noop",
			publish: false,
			remotePaths: remoteBranch?.paths || [],
		};
	}

	let remoteBranch = null;
	if (botBranchExists) {
		refreshRemoteBranch(edge, botBranch);
		remoteBranch = inspectRemoteBranch(edge, {
			mainSha,
			botBranch,
			expectedSha: botBranchSha,
		});
	}
	const candidateMatchesRemote =
		botBranchExists &&
		remoteBranch.baseSha === mainSha &&
		remoteBranch.parentSha === mainSha &&
		matchesRemoteCandidate(edge, botBranch);
	const initialPullRequests = readOpenPullRequests(edge, { repository, defaultBranch, botBranch });
	ensureSinglePullRequest(initialPullRequests, botBranch);

	reconciliation = checkCurrentMain(edge, {
		mainSha,
		defaultBranch,
		phase: candidateMatchesRemote ? "branch reuse" : "candidate publication",
	});
	if (reconciliation) return reconciliation;

	let commitSha;
	if (candidateMatchesRemote) {
		edge.git(["reset", "--hard", mainSha]);
		commitSha = botBranchSha;
	} else {
		edge.git(["config", "user.name", "github-actions[bot]"]);
		edge.git(["config", "user.email", "41898282+github-actions[bot]@users.noreply.github.com"]);
		edge.git(["add", "--", ...SHARED_BASELINE_PATHS]);
		const stagedPaths = parseGitLinePaths(
			edge.git(["diff", "--cached", "--name-only"], { maxBuffer: MAX_METADATA_OUTPUT_BYTES }).stdout,
			"staged generated candidate",
		);
		validateAllowlistedPaths(stagedPaths, { requireChanges: true });
		edge.git(["commit", "-m", prTitle]);
		const generatedCommitSha = edge.git(["rev-parse", "HEAD"], {
			maxBuffer: MAX_METADATA_OUTPUT_BYTES,
		}).stdout.trim();
		requireCommitSha(generatedCommitSha, "generated branch revision");
		verifyCandidateBase(edge, {
			mainSha,
			revision: "HEAD",
			label: "generated candidate",
		});
		reconciliation = checkCurrentMain(edge, {
			mainSha,
			defaultBranch,
			phase: "candidate push",
		});
		if (reconciliation) return reconciliation;
		edge.gh(["auth", "setup-git", "--hostname", "github.com"]);
		const pushArgs = ["push"];
		if (botBranchExists) {
			pushArgs.push(`--force-with-lease=refs/heads/${botBranch}:${botBranchSha}`);
		} else {
			pushArgs.push("--force-with-lease");
		}
		pushArgs.push("origin", `${botBranch}:${botBranch}`);
		edge.git(pushArgs);
		commitSha = generatedCommitSha;
	}

	reconciliation = checkCurrentMain(edge, {
		mainSha,
		defaultBranch,
		phase: "pull request reconciliation",
	});
	if (reconciliation) return reconciliation;

	const pullRequests = readOpenPullRequests(edge, { repository, defaultBranch, botBranch });
	ensureSinglePullRequest(pullRequests, botBranch);
	const prBody = renderPullRequestBody({
		sourceSha: mainSha,
		commitSha,
		runUrl: sourceRunUrl,
		changedPaths: candidatePaths,
	});
	let pullRequestNumber;
	if (pullRequests.length === 1) {
		pullRequestNumber = parsePullRequestNumber(pullRequests[0].number, "gh pr list");
		edge.gh([
			"pr",
			"edit",
			String(pullRequestNumber),
			"--repo",
			repository,
			"--title",
			prTitle,
			"--body",
			prBody,
		]);
	} else {
		const pullRequestURL = edge.gh([
			"pr",
			"create",
			"--repo",
			repository,
			"--base",
			defaultBranch,
			"--head",
			botBranch,
			"--title",
			prTitle,
			"--body",
			prBody,
		]).stdout.trim();
		if (!pullRequestURL) throw new Error("gh pr create did not return a pull request URL");
		pullRequestNumber = parsePullRequestNumber(
			edge.gh([
				"pr",
				"view",
				pullRequestURL,
				"--repo",
				repository,
				"--json",
				"number",
				"--jq",
				".number",
			]).stdout,
			"gh pr view",
		);
	}

	const metadata = parseJsonOutput(
		edge.gh([
			"pr",
			"view",
			String(pullRequestNumber),
			"--repo",
			repository,
			"--json",
			"baseRefName,headRefName,headRefOid,isDraft,files",
		]).stdout,
		"gh pr view",
	);
	validatePullRequestMetadata(metadata, {
		defaultBranch,
		botBranch,
		commitSha,
		candidatePaths,
	});

	reconciliation = checkCurrentMain(edge, {
		mainSha,
		defaultBranch,
		phase: "auto-merge verification",
	});
	if (reconciliation) return reconciliation;

	if (metadata.isDraft === true) {
		edge.gh(["pr", "ready", String(pullRequestNumber), "--repo", repository]);
	}
	edge.gh([
		"pr",
		"merge",
		String(pullRequestNumber),
		"--repo",
		repository,
		"--auto",
		"--squash",
		"--delete-branch",
		"--match-head-commit",
		commitSha,
	]);
	return {
		action: "merge-requested",
		publish: true,
		commitSha,
		pullRequestNumber,
		remotePaths: remoteBranch?.paths || [],
	};
}

function normalizeCommandResult(result) {
	if (typeof result === "string") return { status: 0, stdout: result, stderr: "" };
	return {
		status: result?.status ?? -1,
		stdout: result?.stdout || "",
		stderr: result?.stderr || "",
	};
}

function runExternalCommand(command, args, { allowFailure = false, maxBuffer, stdio } = {}) {
	const spawnOptions = { encoding: "utf8", windowsHide: true };
	if (maxBuffer !== undefined) spawnOptions.maxBuffer = maxBuffer;
	if (stdio !== undefined) spawnOptions.stdio = stdio;
	const raw = spawnSync(command, args, spawnOptions);
	if (raw.error) throw new Error(`unable to execute ${command}: ${raw.error.message}`);
	const result = normalizeCommandResult(raw);
	if (!allowFailure && result.status !== 0) {
		const detail = result.stderr.trim();
		throw new Error(
			`${command} command failed with exit code ${result.status}${detail ? `: ${detail}` : ""}`,
		);
	}
	return result;
}

function runMetadataGit(args) {
	return runExternalCommand("git", args, { maxBuffer: MAX_METADATA_OUTPUT_BYTES }).stdout;
}

function readSourceCommitEvidence(sourceSha) {
	requireCommitSha(sourceSha, "source revision");
	const normalizedSourceSha = String(sourceSha).toLowerCase();
	const resolvedSourceSha = runMetadataGit([
		"rev-parse",
		"--verify",
		`${normalizedSourceSha}^{commit}`,
	]).trim();
	if (resolvedSourceSha.toLowerCase() !== normalizedSourceSha) {
		throw new Error(
			`source revision ${sourceSha} did not resolve to the requested complete commit`,
		);
	}

	const headSha = runMetadataGit(["rev-parse", "--verify", "HEAD"]).trim();
	if (headSha.toLowerCase() !== normalizedSourceSha) {
		throw new Error(
			`checked-out HEAD ${headSha || "(unknown)"} does not match source revision ${sourceSha}`,
		);
	}

	const revisionParts = runMetadataGit([
		"rev-list",
		"--parents",
		"--max-count=1",
		normalizedSourceSha,
	])
		.trim()
		.split(/\s+/)
		.filter(Boolean);
	if (
		(revisionParts.length !== 2 && revisionParts.length !== 3) ||
		revisionParts[0].toLowerCase() !== normalizedSourceSha
	) {
		throw new Error(
			`source revision ${sourceSha} has ambiguous ancestry; expected one parent or a two-parent first-parent merge`,
		);
	}
	const parentSha = revisionParts[1];
	requireCommitSha(parentSha, "source parent revision");
	const sourcePaths = parseGitNameStatusPaths(
		runMetadataGit([
			"diff",
			"--name-status",
			"-z",
			"--find-renames",
			"--find-copies",
			"--find-copies-harder",
			parentSha,
			normalizedSourceSha,
			"--",
		]),
		"source commit diff",
	);
	return { sourceSha: normalizedSourceSha, parentSha, paths: sourcePaths };
}

function readCommittedBlobWithoutPipe(revision, label) {
	let temporaryDirectory = "";
	let outputFileDescriptor;
	let contents;
	let operationError;
	let cleanupError;

	try {
		temporaryDirectory = mkdtempSync(join(tmpdir(), "shared-baseline-blob-"));
		const outputPath = join(temporaryDirectory, "blob");
		outputFileDescriptor = openSync(outputPath, "w");
		runExternalCommand("git", ["show", revision], {
			maxBuffer: MAX_METADATA_OUTPUT_BYTES,
			stdio: ["ignore", outputFileDescriptor, "pipe"],
		});
		const descriptor = outputFileDescriptor;
		outputFileDescriptor = undefined;
		closeSync(descriptor);
		contents = readFileSync(outputPath, "utf8");
	} catch (error) {
		operationError = error;
	}

	if (outputFileDescriptor !== undefined) {
		const descriptor = outputFileDescriptor;
		outputFileDescriptor = undefined;
		try {
			closeSync(descriptor);
		} catch (error) {
			cleanupError = error;
		}
	}
	if (temporaryDirectory) {
		try {
			rmSync(temporaryDirectory, { recursive: true, force: true });
		} catch (error) {
			cleanupError = cleanupError || error;
		}
	}

	if (operationError) {
		if (cleanupError) {
			throw new Error(`${operationError.message}; ${label} temporary cleanup failed: ${cleanupError.message}`);
		}
		throw operationError;
	}
	if (cleanupError) throw new Error(`${label} temporary cleanup failed: ${cleanupError.message}`);
	return contents;
}

function parseBudgetDocument(contents, label) {
	let document;
	try {
		document = JSON.parse(contents);
	} catch (error) {
		throw new Error(`${label} is not valid JSON: ${error.message}`);
	}
	if (
		!document ||
		typeof document !== "object" ||
		Array.isArray(document) ||
		!document.reference ||
		typeof document.reference !== "object" ||
		Array.isArray(document.reference)
	) {
		throw new Error(`${label} has the wrong JSON shape; expected an object with reference.baseCommit`);
	}
	if (
		typeof document.reference.baseCommit !== "string" ||
		!LOWERCASE_COMMIT_SHA_PATTERN.test(document.reference.baseCommit)
	) {
		throw new Error(
			`${label} reference.baseCommit must be a complete lowercase hexadecimal commit SHA`,
		);
	}
	return document;
}

function canonicalJson(value, path = []) {
	if (Array.isArray(value)) {
		return `[${value.map((item, index) => canonicalJson(item, [...path, index])).join(",")}]`;
	}
	if (value && typeof value === "object") {
		return `{${Object.keys(value)
			.filter((key) => !(path.length === 1 && path[0] === "reference" && key === "baseCommit"))
			.sort()
			.map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key], [...path, key])}`)
			.join(",")}}`;
	}
	return JSON.stringify(value);
}

function readUnitBudgetEvidence() {
	const currentContents = readCommittedBlobWithoutPipe(
		`HEAD:${SHARED_BASELINE_UNIT_BUDGET_PATH}`,
		"HEAD unit-latency budget",
	);
	let candidateContents;
	try {
		candidateContents = readFileSync(resolve(SHARED_BASELINE_UNIT_BUDGET_PATH), "utf8");
	} catch (error) {
		throw new Error(`generated unit-latency budget could not be read: ${error.message}`);
	}
	const current = parseBudgetDocument(currentContents, "HEAD unit-latency budget");
	const candidate = parseBudgetDocument(candidateContents, "generated unit-latency budget");
	return {
		matchesExceptBaseCommit:
			canonicalJson(current) === canonicalJson(candidate),
	};
}

function restoreIdentityOnlyBudget() {
	runExternalCommand("git", [
		"restore",
		"--source=HEAD",
		"--staged",
		"--worktree",
		"--",
		SHARED_BASELINE_UNIT_BUDGET_PATH,
	]);
	const remainingPaths = parsePorcelainPaths(
		runMetadataGit(["status", "--porcelain=v1", "--untracked-files=all"]),
	);
	if (remainingPaths.length > 0) {
		throw new Error(
			`identity-only unit-latency candidate was not fully restored; remaining path(s): ${remainingPaths.join(", ")}`,
		);
	}
}

function classifyWorkingTreeCandidate({ sourceEvidence, candidatePaths }) {
	if (candidatePaths.length !== 1 || candidatePaths[0] !== SHARED_BASELINE_UNIT_BUDGET_PATH) {
		return {
			quiescent: false,
			reason: candidatePaths.length === 0
				? "no generated shared baseline changes"
				: "generated candidate contains material or multiple shared baseline changes",
		};
	}

	const budgetEvidence = readUnitBudgetEvidence();
	if (!sourceEvidence.paths.every((path) => SHARED_BASELINE_PATH_SET.has(path))) {
		const sourceOnlyPaths = sourceEvidence.paths.filter((path) => !SHARED_BASELINE_PATH_SET.has(path));
		return {
			quiescent: false,
			reason: `source revision includes non-baseline path(s): ${sourceOnlyPaths.join(", ")}`,
		};
	}
	if (!budgetEvidence.matchesExceptBaseCommit) {
		return {
			quiescent: false,
			reason: "unit-latency budget contains material JSON drift beyond reference.baseCommit",
		};
	}

	restoreIdentityOnlyBudget();
	return {
		quiescent: true,
		reason:
			"source revision changed only allowlisted snapshots and the unit-latency budget differs only at reference.baseCommit; restored the identity-only candidate",
	};
}

function writeGitHubOutput(path, values) {
	if (!path) return;
	for (const [key, value] of Object.entries(values)) {
		appendFileSync(path, `${key}=${value}\n`);
	}
}

function runValidateWorkingTree({
	staged = false,
	requireChanges = false,
	githubOutput = "",
	sourceSha = "",
} = {}) {
	const status = staged
		? runMetadataGit(["diff", "--cached", "--name-only"])
		: runMetadataGit(["status", "--porcelain=v1", "--untracked-files=all"]);
	const paths = staged ? parseGitLinePaths(status, "staged candidate") : parseCandidatePorcelainPaths(status);
	validateAllowlistedPaths(paths, { requireChanges });
	const sourceEvidence = sourceSha ? readSourceCommitEvidence(sourceSha) : null;
	if (!staged && !sourceEvidence) {
		throw new Error("validate-working-tree requires --source-sha with a complete commit SHA");
	}
	const classification = sourceEvidence
		? classifyWorkingTreeCandidate({ sourceEvidence, candidatePaths: paths })
		: {
			quiescent: false,
			reason: paths.length === 0 ? "no staged shared baseline changes" : "staged shared baseline changes retained",
		};
	const changedPaths = classification.quiescent ? [] : paths;
	writeGitHubOutput(githubOutput, {
		changed: changedPaths.length > 0 ? "true" : "false",
		paths: changedPaths.join(","),
		quiescent: classification.quiescent ? "true" : "false",
		reason: classification.reason,
	});
	process.stdout.write(
		`SHARED_BASELINE_CHANGED=${changedPaths.length > 0 ? "true" : "false"} paths=${changedPaths.join(",") || "(none)"} quiescent=${classification.quiescent ? "true" : "false"} reason=${classification.reason}\n`,
	);
}

function parseBooleanEnvironment(value, label) {
	if (value === "true") return true;
	if (value === "false" || value === "") return false;
	throw new Error(`${label} must be true or false`);
}

function runReconcileFromEnvironment() {
	const result = reconcileBotCandidate({
		repository: process.env.REPOSITORY,
		defaultBranch: process.env.DEFAULT_BRANCH || "main",
		botBranch: process.env.BOT_BRANCH || SHARED_BASELINE_BOT_BRANCH,
		prTitle: process.env.SHARED_BASELINE_PR_TITLE || SHARED_BASELINE_PR_TITLE,
		mainSha: process.env.MAIN_SHA,
		botBranchExists: parseBooleanEnvironment(process.env.BOT_BRANCH_EXISTS || "false", "BOT_BRANCH_EXISTS"),
		botBranchSha: process.env.BOT_BRANCH_SHA || "",
		changedPaths: (process.env.CHANGED_PATHS || "").split(",").filter(Boolean),
		sourceRunUrl: process.env.SOURCE_RUN_URL,
	});
	process.stdout.write(
		`SHARED_BASELINE_RECONCILIATION action=${result.action} publish=${result.publish ? "true" : "false"}\n`,
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
			const optionName =
				argument === "--source-sha"
					? "sourceSha"
					: argument === "--commit-sha"
						? "commitSha"
						: "runUrl";
			options[optionName] = rest[++index];
			if (!options[optionName]) throw new Error(`${argument} requires a value`);
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
	if (options.command === "reconcile") {
		runReconcileFromEnvironment();
		return;
	}
	if (options.command === "validate-working-tree") {
		runValidateWorkingTree(options);
		return;
	}
	if (options.command === "list-paths") {
		process.stdout.write(`${SHARED_BASELINE_PATHS.join("\n")}\n`);
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
				sourceSha: options.sourceSha,
				commitSha: options.commitSha,
				runUrl: options.runUrl,
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
