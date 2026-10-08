import { BACKEND_LINT_COMMENT_MARKER } from "./backend-lint-report.mjs";
import { execFileSync } from "node:child_process";
import { appendFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const BACKEND_LINT_EVENTS = Object.freeze(["pull_request", "merge_group", "push"]);
// Keep this positive: the workflow must never pass an empty jobs value to
// Make lint when runner parallelism discovery is unavailable.
export const BACKEND_LINT_FALLBACK_JOBS = 2;
const COMMIT_SHA_PATTERN = /^[0-9a-f]{40}$/i;

const RUNNER_PARALLELISM_MODULE = "./runner-parallelism.mjs";

function describeParallelismError(error) {
	const message = String(error?.message || error || "unknown error")
		.replace(/\s+/g, " ")
		.trim();
	return message.slice(0, 400) || "unknown error";
}

function isNonNegativeSafeInteger(value) {
	return Number.isSafeInteger(value) && value >= 0;
}

function isPositiveSafeInteger(value) {
	return Number.isSafeInteger(value) && value > 0;
}

export async function resolveBackendLintParallelism(
	rawLogicalCPUs,
	loadParallelism = () => import(RUNNER_PARALLELISM_MODULE),
) {
	try {
		const { resolveRunnerParallelism } = await loadParallelism();
		if (typeof resolveRunnerParallelism !== "function") {
			throw new TypeError(
				"runner-parallelism.mjs does not export resolveRunnerParallelism",
			);
		}

		const selection = resolveRunnerParallelism(rawLogicalCPUs);
		if (
			!selection ||
			!isNonNegativeSafeInteger(selection.logicalCPUs) ||
			!isPositiveSafeInteger(selection.jobs)
		) {
			throw new TypeError(
				`runner parallelism calculation returned ${JSON.stringify(selection)}`,
			);
		}

		if (selection.logicalCPUs === 0) {
			return {
				logicalCPUs: 0,
				jobs: BACKEND_LINT_FALLBACK_JOBS,
				warning: `Backend Lint runner parallelism calculation could not determine logical CPUs; using fallback jobs=${BACKEND_LINT_FALLBACK_JOBS}`,
			};
		}

		return { logicalCPUs: selection.logicalCPUs, jobs: selection.logicalCPUs, warning: "" };
	} catch (error) {
		return {
			logicalCPUs: 0,
			jobs: BACKEND_LINT_FALLBACK_JOBS,
			warning: `Backend Lint runner parallelism helper or calculation failed (${describeParallelismError(error)}); using fallback jobs=${BACKEND_LINT_FALLBACK_JOBS}`,
		};
	}
}

export function selectBackendLint({
	eventName = "",
	ref = "",
	sha = "",
} = {}) {
	const selected =
		eventName === "pull_request" ||
		eventName === "merge_group" ||
		(eventName === "push" && ref === "refs/heads/main");
	if (!selected) {
		return {
			selected: false,
			testedSha: "",
			checkoutRef: "",
			error: "",
		};
	}

	const testedSha = String(sha || "").trim();
	const error = COMMIT_SHA_PATTERN.test(testedSha)
		? ""
		: `Backend Lint requires github.sha to be a 40-character commit SHA for ${eventName} events; received ${testedSha || "(empty)"}.`;
	return {
		selected,
		testedSha: error ? "" : testedSha,
		checkoutRef: error ? "" : testedSha,
		error,
	};
}

export function upsertBackendLintComment(comments, body, options = {}) {
	const botLogin = options.botLogin || "github-actions[bot]";
	const marker = options.marker || BACKEND_LINT_COMMENT_MARKER;
	const existing = (comments || []).find(
		(comment) =>
			comment.user?.login === botLogin && comment.body?.includes(marker),
	);
	if (existing) {
		return { action: "update", commentId: existing.id, body };
	}
	return { action: "create", body };
}

// Exclusions are intentionally narrow. Anything not known to be independent
// of compiler/embed or lint inputs selects every optional check.
export function selectLintInputs({ event = "", baseSha = "", testedSha = "", paths = null } = {}) {
	const selection = { version: 1, event, baseSha, testedSha, docs: 1, deadcode: 1, directBoundary: 1,
		reasons: { docs: "conservative", deadcode: "conservative", directBoundary: "conservative" } };
	if (!["pull_request", "merge_group"].includes(event) || !COMMIT_SHA_PATTERN.test(baseSha)
		|| !COMMIT_SHA_PATTERN.test(testedSha) || !Array.isArray(paths) || !paths.length) return selection;
	const isUI = (path) => /^ui\/src\/.+\.(tsx?|css|scss|svg)$/.test(path);
	const isProse = (path) => path === "README.md" || /^docs\/(?!reference\/).+\.md$/.test(path);
	const isGo = (path) => path.endsWith(".go");
	const isDocs = (path) => /^docs\/.+\.md$/.test(path);
	const known = (path) => isUI(path) || isProse(path) || isGo(path) || isDocs(path);
	if (paths.some((path) => typeof path !== "string" || !known(path))) return selection;
	selection.docs = Number(paths.some(isDocs));
	selection.deadcode = Number(paths.some((path) => isGo(path) || path.startsWith("docs/reference/")));
	selection.directBoundary = Number(paths.some((path) => /^(internal\/lint\/|tools\/golangcilintplugin\/)/.test(path)));
	for (const key of ["docs", "deadcode", "directBoundary"]) selection.reasons[key] = selection[key] ? "input" : "unaffected inputs";
	return selection;
}

export function validateLintSelection(selection, identity = {}) {
	const conservative = selection && ["docs", "deadcode", "directBoundary"].every((key) => selection[key] === 1 && selection.reasons?.[key] === "conservative");
	if (!selection || selection.version !== 1 || !BACKEND_LINT_EVENTS.includes(selection.event)
		|| (!COMMIT_SHA_PATTERN.test(selection.baseSha) && !(conservative && selection.baseSha === "")) || !COMMIT_SHA_PATTERN.test(selection.testedSha)
		|| ["docs", "deadcode", "directBoundary"].some((key) => ![0, 1].includes(selection[key])
			|| typeof selection.reasons?.[key] !== "string" || !selection.reasons[key].trim())
		|| (identity.testedSha && selection.testedSha !== identity.testedSha)
		|| (identity.event && selection.event !== identity.event)
		|| (selection.event === "push" && [selection.docs, selection.deadcode, selection.directBoundary].includes(0))) {
		throw new Error("invalid or mismatched lint input selection");
	}
	return selection;
}

export function readLintInputPaths(baseSha, testedSha, git = execFileSync) {
	if (!COMMIT_SHA_PATTERN.test(baseSha) || !COMMIT_SHA_PATTERN.test(testedSha)) return null;
	try {
		// --no-renames reports both old deletion and new addition, including
		// renames out of an input tree; NUL separation preserves unusual names.
		return git("git", ["diff", "--no-renames", "--name-only", "-z", baseSha, testedSha, "--"], { encoding: "utf8" })
			.split("\0").filter(Boolean);
	} catch { return null; }
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	try {
		const args = process.argv.slice(2);
		const value = (name) => { const index = args.indexOf(name); if (index < 0 || args[index + 1] === undefined) throw new Error(`missing ${name}`); return args[index + 1]; };
		if (!args.includes("--select-inputs")) throw new Error("expected --select-inputs");
		const event = value("--event"), baseSha = value("--base"), testedSha = value("--head"), output = value("--output");
		const selection = selectLintInputs({ event, baseSha, testedSha, paths: readLintInputPaths(baseSha, testedSha) });
		// Unavailable Git metadata must still produce an all-selected record.
		mkdirSync(dirname(resolve(output)), { recursive: true });
		writeFileSync(output, JSON.stringify(selection, null, 2) + "\n");
		console.log(JSON.stringify(selection));
		if (process.env.GITHUB_OUTPUT) appendFileSync(process.env.GITHUB_OUTPUT,
			`docs=${selection.docs}\ndeadcode=${selection.deadcode}\ndirect_boundary=${selection.directBoundary}\n`);
	} catch (error) { console.error(error.message); process.exitCode = 1; }
}
