import { BACKEND_LINT_COMMENT_MARKER } from "./backend-lint-report.mjs";

export const BACKEND_LINT_EVENTS = Object.freeze(["pull_request", "push"]);

export function selectBackendLint({
	eventName = "",
	ref = "",
	pullRequestHeadSha = "",
	sha = "",
} = {}) {
	const selected =
		eventName === "pull_request" ||
		(eventName === "push" && ref === "refs/heads/main");
	const headSha = String(pullRequestHeadSha || sha).trim();
	return {
		selected,
		headSha,
		checkoutRef: selected ? headSha : "",
	};
}

export function upsertBackendLintComment(comments, body, options = {}) {
	const botLogin = options.botLogin || "github-actions[bot]";
	const marker = options.marker || BACKEND_LINT_COMMENT_MARKER;
	const existing = (comments || []).find(
		(comment) => comment.user?.login === botLogin && comment.body?.includes(marker),
	);
	if (existing) {
		return { action: "update", commentId: existing.id, body };
	}
	return { action: "create", body };
}
