import { BACKEND_LINT_COMMENT_MARKER } from "./backend-lint-report.mjs";

export const BACKEND_LINT_EVENTS = Object.freeze(["pull_request", "push"]);
// Keep this positive: the workflow must never pass an empty jobs value to
// lintlane when runner parallelism discovery is unavailable.
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

		return { ...selection, warning: "" };
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
