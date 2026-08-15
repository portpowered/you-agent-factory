import assert from "node:assert/strict";
import test from "node:test";

import { evaluateVerificationPolicy } from "../verification-policy.mjs";
import {
	executeLintInventory,
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

test("continues the complete inventory after one checker fails", async () => {
	const started = [];
	const result = await executeLintInventory(["first", "second", "third"], async (target) => {
		started.push(target);
		if (target === "first") {
			throw new Error("controlled checker failure");
		}
		return { output: `${target} passed` };
	});

	assert.deepEqual(started, ["first", "second", "third"]);
	assert.deepEqual(result.targets.map((target) => target.status), ["fail", "pass", "pass"]);
	assert.equal(result.failed[0].error, "controlled checker failure");
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
	assert.match(body, /ui-lint/);
	assert.match(body, /broken-check/);

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
