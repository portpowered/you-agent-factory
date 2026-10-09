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
	selectLintInputs,
	validateLintSelection,
	readLintInputPaths,
	pathsMayBeEmbedded,
} from "./backend-lint-workflow.mjs";
import { resolveRunnerParallelism } from "./runner-parallelism.mjs";

const SHA = (character) => character.repeat(40);

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

test("Backend Lint uses exactly one job on a one-CPU runner without changing other lanes", async () => {
	assert.deepEqual(await resolveBackendLintParallelism("1"), { logicalCPUs: 1, jobs: 1, warning: "" });
	assert.match((await resolveBackendLintParallelism("")).warning, /fallback/);
});

const inputs = (paths, event = "pull_request") => selectLintInputs({ event, baseSha: SHA("a"), testedSha: SHA("b"), paths });

test("queue always selects required deadcode while retaining other input decisions", () => {
	for (const paths of [["README.md"], ["docs/architecture/architecture.md"], ["ui/src/App.tsx"], ["pkg/service/code.go"], ["docs/reference/run.md"]]) {
		const queue = inputs(paths, "merge_group");
		const pr = inputs(paths);
		assert.equal(queue.deadcode, 1);
		assert.equal(queue.reasons.deadcode, "required queue artifact");
		assert.deepEqual([queue.docs, queue.directBoundary], [pr.docs, pr.directBoundary]);
		assert.equal(validateLintSelection(queue), queue);
		assert.throws(() => validateLintSelection({ ...queue, deadcode: 0 }), /selection/);
	}
	for (const paths of [null, [], ["unknown.txt"], [null]]) {
		const queue = inputs(paths, "merge_group");
		assert.deepEqual([queue.docs, queue.deadcode, queue.directBoundary], [1, 1, 1]);
		assert.equal(queue.reasons.deadcode, "conservative");
	}
	const unavailable = selectLintInputs({ event: "merge_group", testedSha: SHA("b") });
	assert.equal(validateLintSelection(unavailable).deadcode, 1);
});

test("only known independent UI/prose changes skip optional checks", () => {
	for (const path of ["README.md", "ui/src/App.tsx"]) {
		const selected = inputs([path]);
		assert.deepEqual([selected.docs, selected.deadcode, selected.directBoundary], [0, 0, 0]);
		assert.equal(validateLintSelection(selected), selected);
	}
	assert.deepEqual([inputs(["docs/architecture/architecture.md"]).docs, inputs(["docs/architecture/architecture.md"]).deadcode], [1, 0]);
	for (const path of ["go.mod", "go.sum", "pkg/service/asset.json", "ui/fallback_dist/index.html", ".golangci.yml", "Makefile", "scripts/deadcode-report.py", "unknown.txt"]) {
		const selected = inputs([path]);
		assert.deepEqual([selected.docs, selected.deadcode, selected.directBoundary], [1, 1, 1], path);
	}
});

test("compiler, embedded docs and analyzer changes select their gates", () => {
	assert.equal(inputs(["pkg/service/code.go"]).deadcode, 1);
	assert.equal(inputs(["docs/reference/run.md"]).deadcode, 1);
	assert.equal(inputs(["docs/reference/run.md"]).docs, 1);
	assert.equal(inputs(["internal/lint/analyzers/code.go"]).directBoundary, 1);
	assert.equal(inputs(["tools/golangcilintplugin/plugin.go"]).directBoundary, 1);
});

test("unknown, empty, missing Git metadata and main push are conservative", () => {
	for (const selection of [inputs([]), inputs(null), inputs(["README.md"], "push"), selectLintInputs(), inputs(["README.md"], "unknown")]) {
		assert.deepEqual([selection.docs, selection.deadcode, selection.directBoundary], [1, 1, 1]);
	}
	assert.equal(readLintInputPaths(SHA("a"), SHA("b"), () => { throw new Error("missing commit"); }), null);
	let argumentsUsed;
	const paths = readLintInputPaths(SHA("a"), SHA("b"), (_, args) => { argumentsUsed = args; return "docs/reference/run.md\0README.md\0"; });
	assert.deepEqual(paths, ["docs/reference/run.md", "README.md"]);
	assert.ok(argumentsUsed.includes("--no-renames"));
	assert.equal(inputs(paths).deadcode, 1);
});

test("selection rejects malformed flags, reasons and mismatched identities", () => {
	const selection = inputs(["README.md"]);
	for (const invalid of [null, {}, { ...selection, deadcode: "0" }, { ...selection, reasons: {} }, { ...selection, baseSha: "" }, { ...selection, event: "push" }]) {
		assert.throws(() => validateLintSelection(invalid), /selection/);
	}
	assert.throws(() => validateLintSelection(selection, { testedSha: SHA("c") }), /selection/);
	assert.throws(() => validateLintSelection(selection, { event: "merge_group" }), /selection/);
});

test("new embeds prevent UI or prose inputs from being skipped", () => {
	const record = (source, patterns) => () => `${SHA("b")}:${source}\0//go:embed ${patterns}\n`;
	assert.equal(pathsMayBeEmbedded(["ui/src/App.tsx"], SHA("b"), record("ui/embed.go", "fallback_dist fallback_dist/*")), false);
	assert.equal(pathsMayBeEmbedded(["ui/src/App.tsx"], SHA("b"), record("ui/embed.go", "all:src")), true);
	assert.equal(pathsMayBeEmbedded(["ui/src/App.tsx"], SHA("b"), record("ui/embed.go", "src/*.tsx")), true);
	assert.equal(pathsMayBeEmbedded(["README.md"], SHA("b"), record("embed.go", "README.md")), true);
	assert.equal(pathsMayBeEmbedded(["ui/src/App.tsx"], SHA("b"), record("ui/embed.go", '"src"')), true);
	assert.equal(pathsMayBeEmbedded(["README.md"], SHA("b"), () => { throw new Error("unavailable metadata"); }), true);
	assert.equal(pathsMayBeEmbedded(["README.md"], SHA("b"), () => { throw { status: 1 }; }), false);
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
