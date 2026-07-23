import assert from "node:assert/strict";
import { access } from "node:fs/promises";
import test from "node:test";

import {
	PackagePublicationError,
	PUBLICATION_FAILURES,
	PUBLICATION_OUTCOMES,
	publishAndVerifyCandidate,
	publishCandidateDirectory,
} from "./api-package-publish.mjs";
import { RECONCILIATION_OUTCOMES } from "./api-package-registry.mjs";

const evidence = {
	packageName: "@you-agent-factory/api",
	candidateVersion: "0.0.0-dev.42.0123456789ab",
	sourceCommit: "0123456789abcdef0123456789abcdef01234567",
	contractDigest: `sha256:${"a".repeat(64)}`,
	artifactDigest: `sha256:${"b".repeat(64)}`,
	inventory: ["generated/manifest.json", "package.json"],
	distTag: "dev",
};

function fixture(overrides = {}) {
	const calls = { install: 0, publish: 0, reconcile: 0, sleep: 0 };
	return {
		calls,
		input: {
			consumerDirectory: "/external/consumer",
			evidence,
			registryClient: {},
			tarballPath: "/candidate/package.tgz",
			workspaceDirectory: "/workspace",
			verificationDelayMs: 1,
		},
		dependencies: {
			async installAndVerifyRegistryPackage(input) {
				calls.install += 1;
				assert.equal(input.candidateVersion, evidence.candidateVersion);
				assert.equal(input.packageName, evidence.packageName);
			},
			async publishCandidateTarball(input) {
				calls.publish += 1;
				assert.equal(input.distTag, "dev");
			},
			async reconcileCandidate() {
				calls.reconcile += 1;
				return { outcome: RECONCILIATION_OUTCOMES.VERIFIED_EXISTING };
			},
			async sleep() {
				calls.sleep += 1;
			},
			...overrides,
		},
	};
}

test("matching retry verifies exact clean consumption without publishing", async () => {
	const subject = fixture();
	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, PUBLICATION_OUTCOMES.VERIFIED_EXISTING);
	assert.deepEqual(subject.calls, {
		install: 1,
		publish: 0,
		reconcile: 1,
		sleep: 0,
	});
});

test("absent candidate publishes once, digest-verifies, and installs exact version", async () => {
	const outcomes = [
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.VERIFIED_EXISTING,
	];
	const subject = fixture({
		async reconcileCandidate() {
			subject.calls.reconcile += 1;
			return { outcome: outcomes.shift() };
		},
	});
	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);

	assert.equal(result.outcome, PUBLICATION_OUTCOMES.PUBLISHED_AND_VERIFIED);
	assert.deepEqual(subject.calls, {
		install: 1,
		publish: 1,
		reconcile: 3,
		sleep: 1,
	});
});

test("immutable conflicts and permission failures stop without publish or install", async () => {
	for (const code of [
		"IMMUTABLE_VERSION_CONFLICT",
		"REGISTRY_PERMISSION_FAILED",
	]) {
		const subject = fixture({
			async reconcileCandidate() {
				subject.calls.reconcile += 1;
				throw new PackagePublicationError(code, "controlled failure");
			},
		});
		await assert.rejects(
			publishAndVerifyCandidate(subject.input, subject.dependencies),
			(error) => error.code === code,
		);
		assert.equal(subject.calls.publish, 0);
		assert.equal(subject.calls.install, 0);
	}
});

test("post-publish verification is bounded and never republishes", async () => {
	const subject = fixture({
		async reconcileCandidate() {
			subject.calls.reconcile += 1;
			return { outcome: RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED };
		},
	});
	subject.input.verificationAttempts = 3;

	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) => error.code === PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED,
	);
	assert.deepEqual(subject.calls, {
		install: 0,
		publish: 1,
		reconcile: 4,
		sleep: 2,
	});
});

test("ambiguous publish failure succeeds only after matching digest verification", async () => {
	const outcomes = [
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.VERIFIED_EXISTING,
	];
	const subject = fixture({
		async publishCandidateTarball() {
			subject.calls.publish += 1;
			throw new PackagePublicationError(
				PUBLICATION_FAILURES.PUBLISH_FAILED,
				"controlled connection loss",
			);
		},
		async reconcileCandidate() {
			subject.calls.reconcile += 1;
			return { outcome: outcomes.shift() };
		},
	});

	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);
	assert.equal(result.outcome, PUBLICATION_OUTCOMES.PUBLISHED_AND_VERIFIED);
	assert.equal(subject.calls.publish, 1);
	assert.equal(subject.calls.reconcile, 2);
	assert.equal(subject.calls.install, 1);
});

test("trusted-publishing permission failure does not retry or verify consumption", async () => {
	const subject = fixture({
		async reconcileCandidate() {
			subject.calls.reconcile += 1;
			return { outcome: RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED };
		},
		async publishCandidateTarball() {
			subject.calls.publish += 1;
			throw new PackagePublicationError(
				PUBLICATION_FAILURES.PERMISSION_FAILED,
				"controlled OIDC permission failure",
			);
		},
	});

	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) => error.code === PUBLICATION_FAILURES.PERMISSION_FAILED,
	);
	assert.equal(subject.calls.publish, 1);
	assert.equal(subject.calls.reconcile, 1);
	assert.equal(subject.calls.install, 0);
});

test("success diagnostics contain only approved evidence and outcome", async () => {
	const subject = fixture();
	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);
	assert.deepEqual(Object.keys(result), [
		"packageName",
		"candidateVersion",
		"sourceCommit",
		"contractDigest",
		"artifactDigest",
		"inventory",
		"distTag",
		"outcome",
	]);
});

test("publish directory loads the preserved candidate and cleans its external consumer", async () => {
	const registryClient = {};
	let consumerDirectory;
	const result = await publishCandidateDirectory(
		{
			candidateDirectory: "/preserved",
			expectedSourceCommit: evidence.sourceCommit,
			workspaceDirectory: "/workspace",
		},
		{
			async candidateFiles(candidateDirectory) {
				assert.equal(candidateDirectory, "/preserved");
				return { evidence, tarballPath: "/preserved/candidate.tgz" };
			},
			async publishAndVerifyCandidate(input) {
				consumerDirectory = input.consumerDirectory;
				await access(consumerDirectory);
				assert.equal(input.evidence, evidence);
				assert.equal(input.registryClient, registryClient);
				assert.equal(input.tarballPath, "/preserved/candidate.tgz");
				assert.equal(input.workspaceDirectory, "/workspace");
				return { outcome: PUBLICATION_OUTCOMES.VERIFIED_EXISTING };
			},
			registryClient,
		},
	);

	assert.equal(result.outcome, PUBLICATION_OUTCOMES.VERIFIED_EXISTING);
	await assert.rejects(access(consumerDirectory));
});

test("publish directory rejects a candidate from a different source commit", async () => {
	let publishCalled = false;
	await assert.rejects(
		publishCandidateDirectory(
			{
				candidateDirectory: "/preserved",
				expectedSourceCommit: "f".repeat(40),
				workspaceDirectory: "/workspace",
			},
			{
				async candidateFiles() {
					return { evidence, tarballPath: "/preserved/candidate.tgz" };
				},
				async publishAndVerifyCandidate() {
					publishCalled = true;
				},
				registryClient: {},
			},
		),
		(error) => error.code === PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED,
	);
	assert.equal(publishCalled, false);
});
