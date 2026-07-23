import assert from "node:assert/strict";
import test from "node:test";

import { executeDevelopmentPackageCommand } from "./api-package-development-command.mjs";
import { DEVELOPMENT_PACKAGE_ACTIONS } from "./api-package-development-policy.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

function fixture(context = {}) {
	const calls = { dryRun: [], prepare: [], publish: [] };
	const candidate = {
		evidence: { sourceCommit, distTag: "dev" },
		evidencePath: "/preserved/candidate-evidence.json",
		tarballPath: "/preserved/candidate.tgz",
	};
	return {
		calls,
		input: {
			action: DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN,
			candidateDirectory: "/preserved",
			eventName: "pull_request",
			outputDirectory: "/preserved",
			packageDirectory: "packages/api",
			prerequisiteResult: "success",
			pullRequestHeadSha: sourceCommit,
			ref: "refs/pull/1160/merge",
			repository: "portpowered/you-agent-factory",
			runId: "42",
			sourceCommit,
			workspaceDirectory: "/workspace",
			...context,
		},
		dependencies: {
			async validatePullRequestCandidate(input) {
				calls.dryRun.push(input);
				return { outcome: "DRY_RUN_NO_PUBLISH" };
			},
			async prepareCandidate(input) {
				calls.prepare.push(input);
				return candidate;
			},
			async publishCandidateDirectory(input) {
				calls.publish.push(input);
				return { outcome: "PUBLISHED_AND_VERIFIED" };
			},
		},
	};
}

test("production pull request command reaches only exact-head dry-run", async () => {
	const subject = fixture();
	const result = await executeDevelopmentPackageCommand(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, "DRY_RUN_NO_PUBLISH");
	assert.deepEqual(subject.calls.dryRun, [
		{
			eventName: "pull_request",
			outputDirectory: "/preserved",
			packageDirectory: "packages/api",
			runId: "42",
			sourceCommit,
			workspaceDirectory: "/workspace",
		},
	]);
	assert.deepEqual(subject.calls.prepare, []);
	assert.deepEqual(subject.calls.publish, []);
});

test("failed prerequisites block every production action before side effects", async () => {
	for (const action of Object.values(DEVELOPMENT_PACKAGE_ACTIONS)) {
		const subject = fixture({
			action,
			eventName:
				action === DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN
					? "pull_request"
					: "push",
			prerequisiteResult: "failure",
			pullRequestHeadSha:
				action === DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN
					? sourceCommit
					: undefined,
			ref:
				action === DEVELOPMENT_PACKAGE_ACTIONS.DRY_RUN
					? "refs/pull/1160/merge"
					: "refs/heads/main",
		});
		await assert.rejects(
			executeDevelopmentPackageCommand(subject.input, subject.dependencies),
			/not allowed for outcome PREREQUISITES_BLOCKED/,
		);
		assert.deepEqual(subject.calls, { dryRun: [], prepare: [], publish: [] });
	}
});

test("production protected-main commands prepare once and publish the preserved directory", async () => {
	const subject = fixture({
		action: DEVELOPMENT_PACKAGE_ACTIONS.PREPARE_MAIN,
		eventName: "push",
		pullRequestHeadSha: undefined,
		ref: "refs/heads/main",
	});
	const prepared = await executeDevelopmentPackageCommand(
		subject.input,
		subject.dependencies,
	);

	assert.equal(prepared.tarballPath, "/preserved/candidate.tgz");
	assert.deepEqual(subject.calls.prepare, [
		{
			outputDirectory: "/preserved",
			packageDirectory: "packages/api",
			runId: "42",
			sourceCommit,
		},
	]);
	const published = await executeDevelopmentPackageCommand(
		{ ...subject.input, action: DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN },
		subject.dependencies,
	);

	assert.equal(published.outcome, "PUBLISHED_AND_VERIFIED");
	assert.deepEqual(subject.calls.publish, [
		{
			candidateDirectory: "/preserved",
			expectedSourceCommit: sourceCommit,
			workspaceDirectory: "/workspace",
		},
	]);
	assert.deepEqual(subject.calls.dryRun, []);
});

test("ineligible production publish commands cannot reach publication", async () => {
	for (const context of [
		{ eventName: "push", ref: "refs/heads/main", repository: "fork/factory" },
		{ eventName: "push", ref: "refs/heads/feature" },
		{ eventName: "push", ref: "refs/tags/v1.0.0" },
	]) {
		const subject = fixture({
			...context,
			action: DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN,
			pullRequestHeadSha: undefined,
		});
		await assert.rejects(
			executeDevelopmentPackageCommand(subject.input, subject.dependencies),
			/not allowed for outcome EVENT_INELIGIBLE/,
		);
		assert.deepEqual(subject.calls, { dryRun: [], prepare: [], publish: [] });
	}
});

test("production pull request command rejects a commit other than the reviewed head", async () => {
	const subject = fixture({ sourceCommit: "f".repeat(40) });
	await assert.rejects(
		executeDevelopmentPackageCommand(subject.input, subject.dependencies),
		/reviewed head SHA/,
	);
	assert.deepEqual(subject.calls, { dryRun: [], prepare: [], publish: [] });
});
