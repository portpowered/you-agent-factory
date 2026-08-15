import assert from "node:assert/strict";
import test from "node:test";

import { evaluateVerificationPolicy } from "../verification-policy.mjs";
import {
	selectBackendLint,
	upsertBackendLintComment,
} from "./backend-lint-workflow.mjs";
import {
	BACKEND_LINT_COMMENT_MARKER,
	renderBackendLintComment,
	summarizeBackendLintReport,
} from "./backend-lint-report.mjs";

test("selects pull requests and pushes to main at the tested head", () => {
	assert.deepEqual(
		selectBackendLint({
			eventName: "pull_request",
			ref: "refs/pull/42/merge",
			pullRequestHeadSha: "pr-head",
			sha: "merge-sha",
		}),
		{ selected: true, headSha: "pr-head", checkoutRef: "pr-head" },
	);
	assert.deepEqual(
		selectBackendLint({
			eventName: "push",
			ref: "refs/heads/main",
			sha: "main-head",
		}),
		{ selected: true, headSha: "main-head", checkoutRef: "main-head" },
	);
	assert.equal(
		selectBackendLint({ eventName: "push", ref: "refs/heads/feature", sha: "feature-head" }).selected,
		false,
	);
});

test("publishes a complete report by creating or updating the marked bot comment", () => {
	const body = renderBackendLintComment(
		summarizeBackendLintReport({
			version: 1,
			jobs: 2,
			totalDurationMillis: 2500,
			targets: [
				{ name: "ui-lint", status: "pass", durationMillis: 1000, output: "pass" },
				{
					name: "broken-check",
					status: "fail",
					durationMillis: 1500,
					violationCount: 1,
					output: "LINT_VIOLATION_COUNT: 1\nchecker output",
				},
			],
		}),
		{ headSha: "tested-head", runUrl: "https://example.test/run/1" },
	);
	assert.match(body, new RegExp(BACKEND_LINT_COMMENT_MARKER.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
	assert.match(body, /\| ui-lint \| `pass` \| 0 \| 1\.00s \| clean \|/);
	assert.match(body, /\| broken-check \| `fail` \| 1 \| 1\.50s \| new failure \|/);
	assert.match(body, /Total Backend Lint wall time: `2\.50s`/);

	assert.deepEqual(upsertBackendLintComment([], body), { action: "create", body });
	assert.deepEqual(
		upsertBackendLintComment(
			[
				{ id: 17, user: { login: "reviewer" }, body },
				{ id: 19, user: { login: "github-actions[bot]" }, body: `${BACKEND_LINT_COMMENT_MARKER}\nold` },
			],
			body,
		),
		{ action: "update", commentId: 19, body },
	);
});

test("required-result propagation rejects every non-success Backend Lint result", () => {
	const policyFor = (result) =>
		evaluateVerificationPolicy({
			classificationResult: "success",
			classification: "full",
			packageWorkflowResult: "success",
			lanes: [
				{
					name: "Backend Lint",
					selected: "true",
					reason: "The canonical lint inventory is required.",
					checks: [{ name: "Backend Lint", result }],
				},
			],
		});

	assert.equal(policyFor("success").ok, true);
	for (const result of ["skipped", "cancelled", "timed_out", "failure"]) {
		assert.equal(policyFor(result).ok, false, `${result} must fail the required lane`);
	}
});
