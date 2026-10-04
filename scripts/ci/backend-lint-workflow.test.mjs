import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
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

const SHA = (character) => character.repeat(40);

test("complete lint inventory keeps positive concurrency after an earlier step fails", () => {
	const workflow = readFileSync(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
	const selector = workflow.split("      - name: Select Backend Lint runner parallelism")[1]?.split("      - name:")[0];
	assert.match(selector ?? "", /\n\s+if: always\(\)/);
	const inventory = workflow.split("      - name: Run complete canonical Backend Lint inventory")[1]?.split("      - name:")[0];
	assert.match(inventory ?? "", /\n\s+if: always\(\)/);
	assert.match(inventory ?? "", new RegExp(`LINT_JOBS: \\$\\{\\{ steps\\.backend-lint-parallelism\\.outputs\\.jobs \\|\\| '${BACKEND_LINT_FALLBACK_JOBS}' \\}\\}`));
});

test("selects pull requests at the merge result and pushes to main at the tested commit", () => {
	assert.deepEqual(
		selectBackendLint({
			eventName: "pull_request",
			ref: "refs/pull/42/merge",
			pullRequestHeadSha: SHA("b"),
			sha: SHA("a"),
		}),
		{ selected: true, testedSha: SHA("a"), checkoutRef: SHA("a"), error: "" },
	);
	assert.deepEqual(
		selectBackendLint({
			eventName: "push",
			ref: "refs/heads/main",
			sha: SHA("b"),
		}),
		{ selected: true, testedSha: SHA("b"), checkoutRef: SHA("b"), error: "" },
	);
	assert.deepEqual(
		selectBackendLint({ eventName: "push", ref: "refs/heads/feature", sha: SHA("c") }),
		{ selected: false, testedSha: "", checkoutRef: "", error: "" },
	);
});

test("fails selected events when the required tested identity is missing or invalid", () => {
	for (const sha of ["", "not-a-commit-sha", "0".repeat(39)]) {
		const selection = selectBackendLint({
			eventName: "pull_request",
			ref: "refs/pull/42/merge",
			sha,
		});
		assert.equal(selection.selected, true);
		assert.equal(selection.testedSha, "");
		assert.equal(selection.checkoutRef, "");
		assert.match(selection.error, /requires github\.sha/);
	}
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

test("selects merge queue groups at the tested merge group commit", () => {
	assert.deepEqual(
		selectBackendLint({
			eventName: "merge_group",
			ref: "refs/heads/gh-readonly-queue/main/pr-42-abc",
			sha: SHA("d"),
		}),
		{ selected: true, testedSha: SHA("d"), checkoutRef: SHA("d"), error: "" },
	);
	assert.match(
		selectBackendLint({ eventName: "merge_group", ref: "", sha: "" }).error,
		/requires github\.sha/,
	);
});

test("required checks report on merge_group using merge group base and head SHAs", () => {
	const workflow = readFileSync(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
	assert.match(workflow, /\n  merge_group:\r?\n    types: \[checks_requested\]/);
	assert.match(
		workflow,
		/-base "\$\{\{ github\.event\.pull_request\.base\.sha \|\| github\.event\.merge_group\.base_sha \}\}" -head "\$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.event\.merge_group\.head_sha \}\}"/,
	);
	assert.match(workflow, /github\.event_name == 'pull_request' \|\| github\.event_name == 'merge_group'\r?\n\s+run: go run \.\/cmd\/ciclassify/);
	assert.match(workflow, /if: github\.event_name != 'pull_request' && github\.event_name != 'merge_group'/);
	const lint = workflow.split("\n  backend-lint:")[1]?.split("\n  ui-backend-integration:")[0] ?? "";
	assert.match(lint, /github\.event_name == 'merge_group'/);
	const pkg = workflow.split("\n  development-package:")[1]?.split("\n  development-package-behavior:")[0] ?? "";
	assert.match(pkg, /github\.event_name == 'merge_group'/);
	assert.match(pkg, /run_candidates: \$\{\{ github\.event_name == 'pull_request' \}\}/);
});
