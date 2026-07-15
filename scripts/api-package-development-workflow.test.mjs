import assert from "node:assert/strict";
import test from "node:test";

import {
	DEVELOPMENT_PACKAGE_OUTCOMES,
	executeDevelopmentPackagePolicy,
} from "./api-package-development-policy.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

function fixture(context = {}, prerequisiteResult = "success") {
	const calls = { dryRun: [], prepare: [], publish: [], prerequisites: 0 };
	const candidate = {
		evidence: { sourceCommit, distTag: "dev" },
		tarballPath: "/preserved/candidate.tgz",
	};
	return {
		calls,
		input: {
			eventName: "pull_request",
			outputDirectory: "/preserved",
			packageDirectory: "packages/api",
			pullRequestHeadSha: sourceCommit,
			ref: "refs/pull/1160/merge",
			repository: "portpowered/you-agent-factory",
			runId: "42",
			sourceCommit,
			workspaceDirectory: "/workspace",
			...context,
		},
		dependencies: {
			async runPrerequisites() {
				calls.prerequisites += 1;
				return prerequisiteResult;
			},
			async validatePullRequestCandidate(input) {
				calls.dryRun.push(input);
				return { outcome: "DRY_RUN_NO_PUBLISH" };
			},
			async prepareCandidate(input) {
				calls.prepare.push(input);
				return candidate;
			},
			async publishAndVerifyCandidate(input) {
				calls.publish.push(input);
				return { outcome: "PUBLISHED_AND_VERIFIED" };
			},
		},
	};
}

test("pull request success reaches only the exact-head dry-run boundary", async () => {
	const subject = fixture();
	const result = await executeDevelopmentPackagePolicy(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, "DRY_RUN_NO_PUBLISH");
	assert.equal(subject.calls.prerequisites, 1);
	assert.equal(subject.calls.dryRun.length, 1);
	assert.equal(subject.calls.dryRun[0].sourceCommit, sourceCommit);
	assert.equal(subject.calls.dryRun[0].outputDirectory, "/preserved");
	assert.deepEqual(subject.calls.prepare, []);
	assert.deepEqual(subject.calls.publish, []);
});

test("failed prerequisites block every candidate and publication boundary", async () => {
	const subject = fixture({}, "failure");
	const result = await executeDevelopmentPackagePolicy(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, DEVELOPMENT_PACKAGE_OUTCOMES.BLOCKED);
	assert.deepEqual(subject.calls.dryRun, []);
	assert.deepEqual(subject.calls.prepare, []);
	assert.deepEqual(subject.calls.publish, []);
});

test("protected main prepares once and hands that preserved candidate to publish and verify", async () => {
	const subject = fixture({
		eventName: "push",
		pullRequestHeadSha: undefined,
		ref: "refs/heads/main",
	});
	const result = await executeDevelopmentPackagePolicy(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, "PUBLISHED_AND_VERIFIED");
	assert.equal(subject.calls.prepare.length, 1);
	assert.deepEqual(subject.calls.prepare[0], {
		outputDirectory: "/preserved",
		packageDirectory: "packages/api",
		runId: "42",
		sourceCommit,
	});
	assert.equal(subject.calls.publish.length, 1);
	assert.equal(subject.calls.publish[0].tarballPath, "/preserved/candidate.tgz");
	assert.equal(subject.calls.publish[0].evidence.distTag, "dev");
	assert.equal(subject.calls.publish[0].evidence.sourceCommit, sourceCommit);
	assert.equal(subject.calls.publish[0].workspaceDirectory, "/workspace");
	assert.deepEqual(subject.calls.dryRun, []);
});

test("fork pushes, other branches, and tags never reach OIDC-authorized publication", async () => {
	for (const context of [
		{ eventName: "push", ref: "refs/heads/main", repository: "fork/factory" },
		{ eventName: "push", ref: "refs/heads/feature" },
		{ eventName: "push", ref: "refs/tags/v1.0.0" },
	]) {
		const subject = fixture({ ...context, pullRequestHeadSha: undefined });
		const result = await executeDevelopmentPackagePolicy(
			subject.input,
			subject.dependencies,
		);

		assert.equal(result.outcome, DEVELOPMENT_PACKAGE_OUTCOMES.INELIGIBLE);
		assert.deepEqual(subject.calls.dryRun, []);
		assert.deepEqual(subject.calls.prepare, []);
		assert.deepEqual(subject.calls.publish, []);
	}
});

test("pull request candidate handoff rejects a commit other than the reviewed head", async () => {
	const subject = fixture({ sourceCommit: "f".repeat(40) });
	await assert.rejects(
		executeDevelopmentPackagePolicy(subject.input, subject.dependencies),
		/reviewed head SHA/,
	);
	assert.deepEqual(subject.calls.dryRun, []);
	assert.deepEqual(subject.calls.publish, []);
});
