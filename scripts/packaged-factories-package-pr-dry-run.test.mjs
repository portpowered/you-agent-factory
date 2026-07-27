import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	DRY_RUN_OUTCOME,
	validatePullRequestCandidate,
} from "./packaged-factories-package-pr-dry-run.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

async function temporaryDirectory(t, name) {
	const directory = await mkdtemp(join(tmpdir(), name));
	t.after(() => rm(directory, { recursive: true, force: true }));
	return directory;
}

function input(outputDirectory, overrides = {}) {
	return {
		eventName: "pull_request",
		outputDirectory,
		packageDirectory: "packages/packaged-factories",
		prerequisiteResult: undefined,
		pullRequestHeadSha: sourceCommit,
		ref: "refs/pull/42/merge",
		repository: "portpowered/you-agent-factory",
		runId: "42",
		sourceCommit,
		workspaceDirectory: process.cwd(),
		...overrides,
	};
}

test("pull request dry run verifies and preserves the exact reviewed candidate", async (t) => {
	const outputDirectory = await temporaryDirectory(
		t,
		"you-packaged-factories-pr-",
	);
	const tarballPath = join(outputDirectory, "candidate.tgz");
	const evidence = {
		packageName: "@you-agent-factory/packaged-factories",
		candidateVersion: "0.0.0-dev.42.0123456789ab",
		sourceCommit,
		contractDigest: `sha256:${"a".repeat(64)}`,
		artifactDigest: `sha256:${"b".repeat(64)}`,
		inventory: ["generated/manifest.json", "package.json"],
		distTag: "dev",
	};
	const consumerEvidence = {
		packageName: evidence.packageName,
		packageVersion: evidence.candidateVersion,
		sourceCommit,
		factoryCount: 7,
	};
	let verified;

	const result = await validatePullRequestCandidate(input(outputDirectory), {
		prepareCandidate: async (received) => {
			assert.equal(received.sourceCommit, sourceCommit);
			return { evidence, tarballPath };
		},
		installAndVerifyTarball: async (received) => {
			verified = received;
			return consumerEvidence;
		},
	});

	assert.deepEqual(verified, {
		expectedSourceCommit: sourceCommit,
		expectedVersion: evidence.candidateVersion,
		packageName: evidence.packageName,
		tarballPath,
		workspaceDirectory: process.cwd(),
	});
	assert.deepEqual(
		JSON.parse(
			await readFile(
				join(outputDirectory, result.consumerEvidenceFile),
				"utf8",
			),
		),
		consumerEvidence,
	);
	assert.equal(result.outcome, DRY_RUN_OUTCOME);
	assert.equal(result.sourceCommit, sourceCommit);
});

test("pull request authorization rejects non-reviewed identity before preparation", async (t) => {
	const outputDirectory = await temporaryDirectory(
		t,
		"you-packaged-factories-pr-rejected-",
	);
	for (const overrides of [
		{ sourceCommit: "f".repeat(40) },
		{ sourceCommit: undefined },
		{ eventName: "push" },
	]) {
		let prepared = false;
		await assert.rejects(
			validatePullRequestCandidate(input(outputDirectory, overrides), {
				prepareCandidate: async () => {
					prepared = true;
				},
			}),
			/reviewed head SHA|source commit is required|not allowed/,
		);
		assert.equal(prepared, false);
	}
});

test("dry-run diagnostics identify candidate and consumer failure stages", async (t) => {
	const stages = [
		[
			"generation",
			"[packaged-factories-package-pack] generated catalog drift check failed",
		],
		["identity", "[package-release-candidate] source commit is required"],
		[
			"inventory",
			"[packaged-factories-package-pack] tarball inventory rejected",
		],
		["pack", "[packaged-factories-package-pack] npm pack failed"],
	];
	for (const [stage, message] of stages) {
		const outputDirectory = await temporaryDirectory(
			t,
			`you-packaged-factories-pr-${stage}-`,
		);
		await assert.rejects(
			validatePullRequestCandidate(input(outputDirectory), {
				prepareCandidate: async () => {
					throw new Error(message);
				},
			}),
			new RegExp(`${stage} stage failed`),
		);
	}

	const outputDirectory = await temporaryDirectory(
		t,
		"you-packaged-factories-pr-consumer-",
	);
	await assert.rejects(
		validatePullRequestCandidate(input(outputDirectory), {
			prepareCandidate: async () => ({
				evidence: {
					candidateVersion: "0.0.0-dev.42.0123456789ab",
					packageName: "@you-agent-factory/packaged-factories",
				},
				tarballPath: join(outputDirectory, "candidate.tgz"),
			}),
			installAndVerifyTarball: async () => {
				throw new Error("schema validation failed");
			},
		}),
		/installed-consumer stage failed: schema validation failed/,
	);
});
