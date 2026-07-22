import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	DRY_RUN_OUTCOME,
	validatePullRequestCandidate,
} from "./api-package-pr-dry-run.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

test("pull request dry run verifies the exact prepared candidate without a registry boundary", async (t) => {
	const outputDirectory = await temporaryDirectory(t, "you-api-pr-output-");
	const tarballPath = join(outputDirectory, "candidate.tgz");
	const evidence = {
		packageName: "@you-agent-factory/api",
		candidateVersion: "0.0.2-dev.42.0123456789ab",
		sourceCommit,
		contractDigest: `sha256:${"a".repeat(64)}`,
		artifactDigest: `sha256:${"b".repeat(64)}`,
		inventory: ["generated/manifest.json", "package.json"],
		distTag: "dev",
	};
	let receivedCandidate;

	const result = await validatePullRequestCandidate(
		{
			eventName: "pull_request",
			outputDirectory,
			packageDirectory: "packages/api",
			runId: "42",
			sourceCommit,
			workspaceDirectory: process.cwd(),
		},
		{
			prepareCandidate: async (input) => {
				assert.equal(input.outputDirectory, outputDirectory);
				return { evidence, tarballPath };
			},
			installAndVerifyTarball: async (input) => {
				receivedCandidate = input;
			},
		},
	);

	assert.equal(receivedCandidate.tarballPath, tarballPath);
	assert.equal(receivedCandidate.packageName, evidence.packageName);
	assert.deepEqual(receivedCandidate.packedFiles, evidence.inventory);
	assert.equal(receivedCandidate.workspaceDirectory, process.cwd());
	assert.deepEqual(result, { ...evidence, outcome: DRY_RUN_OUTCOME });
});

test("non-pull-request events fail before candidate preparation", async (t) => {
	const outputDirectory = await temporaryDirectory(t, "you-api-pr-rejected-");
	let prepared = false;

	await assert.rejects(
		validatePullRequestCandidate(
			{
				eventName: "push",
				outputDirectory,
				packageDirectory: "packages/api",
				runId: "42",
				sourceCommit,
				workspaceDirectory: process.cwd(),
			},
			{
				prepareCandidate: async () => {
					prepared = true;
				},
			},
		),
		/only pull_request events may use the dry-run path/,
	);
	assert.equal(prepared, false);
});

test("consumer failure prevents a dry-run success outcome", async (t) => {
	const outputDirectory = await temporaryDirectory(t, "you-api-pr-failure-");

	await assert.rejects(
		validatePullRequestCandidate(
			{
				eventName: "pull_request",
				outputDirectory,
				packageDirectory: "packages/api",
				runId: "42",
				sourceCommit,
				workspaceDirectory: process.cwd(),
			},
			{
				prepareCandidate: async () => ({
					evidence: {
						packageName: "@you-agent-factory/api",
						inventory: ["package.json"],
					},
					tarballPath: join(outputDirectory, "candidate.tgz"),
				}),
				installAndVerifyTarball: async () => {
					throw new Error("semantic export verification failed");
				},
			},
		),
		/semantic export verification failed/,
	);
});
