import assert from "node:assert/strict";
import test from "node:test";
import { DEVELOPMENT_PACKAGE_ACTIONS } from "./package-development-policy.mjs";
import { executePackagedFactoriesDevelopmentCommand } from "./packaged-factories-package-development-command.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";
const protectedMain = {
	eventName: "push",
	prerequisiteResult: "success",
	ref: "refs/heads/main",
	repository: "portpowered/you-agent-factory",
	sourceCommit,
};

test("protected main prepares and publishes the preserved Packaged Factories candidate", async () => {
	const calls = [];
	const dependencies = {
		async prepareCandidate(input) {
			calls.push(["prepare", input]);
			return { evidence: { sourceCommit } };
		},
		async publishCandidateDirectory(input) {
			calls.push(["publish", input]);
			return { outcome: "VERIFIED_EXISTING" };
		},
	};
	await executePackagedFactoriesDevelopmentCommand(
		{
			...protectedMain,
			action: DEVELOPMENT_PACKAGE_ACTIONS.PREPARE_MAIN,
			outputDirectory: "/candidate",
			packageDirectory: "/package",
			runId: "42",
		},
		dependencies,
	);
	await executePackagedFactoriesDevelopmentCommand(
		{
			...protectedMain,
			action: DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN,
			candidateDirectory: "/candidate",
			workspaceDirectory: "/workspace",
		},
		dependencies,
	);
	assert.deepEqual(calls, [
		[
			"prepare",
			{
				outputDirectory: "/candidate",
				packageDirectory: "/package",
				runId: "42",
				sourceCommit,
			},
		],
		[
			"publish",
			{
				candidateDirectory: "/candidate",
				expectedSourceCommit: sourceCommit,
				workspaceDirectory: "/workspace",
			},
		],
	]);
});

test("pull requests and unprotected repositories cannot publish Packaged Factories", async () => {
	for (const input of [
		{
			...protectedMain,
			eventName: "pull_request",
			pullRequestHeadSha: sourceCommit,
		},
		{ ...protectedMain, repository: "portpowered/infinite-you" },
	]) {
		let published = false;
		await assert.rejects(
			executePackagedFactoriesDevelopmentCommand(
				{
					...input,
					action: DEVELOPMENT_PACKAGE_ACTIONS.PUBLISH_MAIN,
				},
				{
					async publishCandidateDirectory() {
						published = true;
					},
				},
			),
			/not allowed/,
		);
		assert.equal(published, false);
	}
});
