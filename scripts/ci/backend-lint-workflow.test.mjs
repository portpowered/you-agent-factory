import assert from "node:assert/strict";
import test from "node:test";

import { evaluateVerificationPolicy } from "../verification-policy.mjs";
import {
	BACKEND_LINT_COMMENT_MARKER,
	renderBackendLintComment,
	summarizeBackendLintReport,
} from "./backend-lint-report.mjs";
import {
	BACKEND_LINT_FALLBACK_JOBS,
	resolveBackendLintParallelism,
	selectBackendLint,
	upsertBackendLintComment,
} from "./backend-lint-workflow.mjs";
import { resolveRunnerParallelism } from "./runner-parallelism.mjs";

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

test("uses all valid logical CPUs for the exclusive CI runner", () => {
	assert.deepEqual(resolveRunnerParallelism("4"), { logicalCPUs: 4, jobs: 4 });
	assert.deepEqual(resolveRunnerParallelism(" 8\n"), { logicalCPUs: 8, jobs: 8 });
});

test("uses the safe minimum when logical CPU discovery is invalid or unavailable", () => {
	for (const rawLogicalCPUs of ["", "not-a-number", "0", "4x"]) {
		assert.deepEqual(
			resolveRunnerParallelism(rawLogicalCPUs),
			{ logicalCPUs: 0, jobs: 2 },
			`raw logical CPU value ${JSON.stringify(rawLogicalCPUs)}`,
		);
	}
	assert.deepEqual(resolveRunnerParallelism("1"), { logicalCPUs: 1, jobs: 2 });
});

test("uses the healthy runner-parallelism selection when the helper loads", async () => {
	assert.deepEqual(await resolveBackendLintParallelism("8"), {
		logicalCPUs: 8,
		jobs: 8,
		warning: "",
	});
});

test("exports a positive fallback and warning when the helper cannot load", async () => {
	const result = await resolveBackendLintParallelism("8", async () => {
		throw new Error("ERR_MODULE_NOT_FOUND: runner-parallelism.mjs");
	});

	assert.equal(result.logicalCPUs, 0);
	assert.equal(result.jobs, BACKEND_LINT_FALLBACK_JOBS);
	assert.ok(result.jobs > 0);
	assert.match(
		result.warning,
		/runner parallelism helper or calculation failed/,
	);
	assert.match(result.warning, /ERR_MODULE_NOT_FOUND: runner-parallelism\.mjs/);
	assert.match(
		result.warning,
		new RegExp(`fallback jobs=${BACKEND_LINT_FALLBACK_JOBS}`),
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
	assert.match(body, /\| ui-lint \| `pass` \| 0 \| 0 \| \+0 \| 1\.00s \| clean \|/);
	assert.match(body, /\| broken-check \| `fail` \| 0 \| 1 \| \+1 \| 1\.50s \| new failure \|/);
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
