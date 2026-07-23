import assert from "node:assert/strict";
import test from "node:test";

import {
	PackagePublicationError,
	PUBLICATION_FAILURES,
	PUBLICATION_OUTCOMES,
	publishAndVerifyCandidate,
} from "./packaged-factories-package-publish.mjs";
import {
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
} from "./packaged-factories-package-registry.mjs";

const evidence = {
	packageName: "@you-agent-factory/packaged-factories",
	candidateVersion: "0.0.0-dev.42.0123456789ab",
	sourceCommit: "0123456789abcdef0123456789abcdef01234567",
	contractDigest: `sha256:${"a".repeat(64)}`,
	artifactDigest: `sha256:${"b".repeat(64)}`,
	inventory: ["generated/manifest.json", "package.json"],
	distTag: "dev",
};

function fixture(outcomes) {
	const calls = { install: 0, publish: 0, reconcile: 0, sleep: 0 };
	return {
		calls,
		input: {
			consumerDirectory: "/external/consumer",
			evidence,
			registryClient: {},
			tarballPath: "/candidate/package.tgz",
			verificationDelayMs: 1,
			workspaceDirectory: "/workspace",
		},
		dependencies: {
			async installAndVerifyRegistryPackage(input) {
				calls.install += 1;
				assert.equal(input.packageName, evidence.packageName);
				assert.equal(input.candidateVersion, evidence.candidateVersion);
				assert.equal(input.expectedSourceCommit, evidence.sourceCommit);
			},
			async publishCandidateTarball(input) {
				calls.publish += 1;
				assert.equal(input.distTag, "dev");
			},
			async reconcileCandidate() {
				calls.reconcile += 1;
				const outcome = outcomes.shift();
				if (outcome instanceof Error) {
					throw outcome;
				}
				return { outcome };
			},
			async sleep() {
				calls.sleep += 1;
			},
		},
	};
}

function registryFailure(code) {
	return new RegistryReconciliationError(code, "controlled registry failure");
}

test("an identical Packaged Factories registry version is consumed without republish", async () => {
	const subject = fixture([RECONCILIATION_OUTCOMES.VERIFIED_EXISTING]);
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

test("transient initial registry lookup failure recovers within the bounded window", async () => {
	const subject = fixture([
		registryFailure(RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED),
		RECONCILIATION_OUTCOMES.VERIFIED_EXISTING,
	]);
	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);
	assert.equal(result.outcome, PUBLICATION_OUTCOMES.VERIFIED_EXISTING);
	assert.deepEqual(subject.calls, {
		install: 1,
		publish: 0,
		reconcile: 2,
		sleep: 1,
	});
});

test("transient post-publish registry download failure recovers without republishing", async () => {
	const subject = fixture([
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		registryFailure(RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED),
		RECONCILIATION_OUTCOMES.VERIFIED_EXISTING,
	]);
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

test("transient registry consumer install failure recovers within the bounded window", async () => {
	const subject = fixture([RECONCILIATION_OUTCOMES.VERIFIED_EXISTING]);
	subject.dependencies.installAndVerifyRegistryPackage = async () => {
		subject.calls.install += 1;
		if (subject.calls.install === 1) {
			throw registryFailure(RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED);
		}
	};
	const result = await publishAndVerifyCandidate(
		subject.input,
		subject.dependencies,
	);
	assert.equal(result.outcome, PUBLICATION_OUTCOMES.VERIFIED_EXISTING);
	assert.deepEqual(subject.calls, {
		install: 2,
		publish: 0,
		reconcile: 1,
		sleep: 1,
	});
});

test("transient initial lookup exhaustion preserves the classified diagnostic", async () => {
	const subject = fixture([
		registryFailure(RECONCILIATION_FAILURES.REGISTRY_TIMEOUT),
		registryFailure(RECONCILIATION_FAILURES.REGISTRY_TIMEOUT),
	]);
	subject.input.verificationAttempts = 2;
	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) => error.code === RECONCILIATION_FAILURES.REGISTRY_TIMEOUT,
	);
	assert.deepEqual(subject.calls, {
		install: 0,
		publish: 0,
		reconcile: 2,
		sleep: 1,
	});
});

test("identity, digest, conflict, authentication, and permission failures do not retry", async () => {
	for (const code of [
		RECONCILIATION_FAILURES.CANDIDATE_INVALID,
		RECONCILIATION_FAILURES.CANDIDATE_DIGEST_VERIFICATION_FAILED,
		RECONCILIATION_FAILURES.IMMUTABLE_VERSION_CONFLICT,
		RECONCILIATION_FAILURES.REGISTRY_INTEGRITY_FAILED,
		RECONCILIATION_FAILURES.REGISTRY_AUTHENTICATION_FAILED,
		RECONCILIATION_FAILURES.REGISTRY_PERMISSION_FAILED,
	]) {
		const subject = fixture([registryFailure(code)]);
		subject.input.verificationAttempts = 3;
		await assert.rejects(
			publishAndVerifyCandidate(subject.input, subject.dependencies),
			(error) => error.code === code,
		);
		assert.deepEqual(subject.calls, {
			install: 0,
			publish: 0,
			reconcile: 1,
			sleep: 0,
		});
	}
});

test("registry consumer exhaustion reports bounded verification and its cause", async () => {
	const subject = fixture([RECONCILIATION_OUTCOMES.VERIFIED_EXISTING]);
	subject.input.verificationAttempts = 2;
	subject.dependencies.installAndVerifyRegistryPackage = async () => {
		subject.calls.install += 1;
		throw registryFailure(RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED);
	};
	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) =>
			error.code === PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED &&
			error.cause?.code === RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED &&
			/within 2 attempts/.test(error.message),
	);
	assert.deepEqual(subject.calls, {
		install: 2,
		publish: 0,
		reconcile: 1,
		sleep: 1,
	});
});

test("an absent Packaged Factories version is published once then verified and consumed", async () => {
	const subject = fixture([
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.VERIFIED_EXISTING,
	]);
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

test("Packaged Factories publication bounds visibility retries and never republishes", async () => {
	const subject = fixture([
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
		RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED,
	]);
	subject.input.verificationAttempts = 2;
	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) =>
			error.code === PUBLICATION_FAILURES.REGISTRY_VERIFICATION_FAILED &&
			error.cause?.code === RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED &&
			/within 2 attempts/.test(error.message),
	);
	assert.deepEqual(subject.calls, {
		install: 0,
		publish: 1,
		reconcile: 3,
		sleep: 1,
	});
});

test("Packaged Factories trusted-publishing permission failure stops immediately", async () => {
	const subject = fixture([RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED]);
	subject.dependencies.publishCandidateTarball = async () => {
		subject.calls.publish += 1;
		throw new PackagePublicationError(
			PUBLICATION_FAILURES.PERMISSION_FAILED,
			"controlled permission failure",
		);
	};
	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) => error.code === PUBLICATION_FAILURES.PERMISSION_FAILED,
	);
	assert.equal(subject.calls.reconcile, 1);
	assert.equal(subject.calls.publish, 1);
	assert.equal(subject.calls.install, 0);
});

test("Packaged Factories publication reports a distinct publish timeout", async () => {
	const subject = fixture([RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED]);
	subject.input.verificationAttempts = 1;
	subject.dependencies.publishCandidateTarball = async () => {
		subject.calls.publish += 1;
		throw new PackagePublicationError(
			PUBLICATION_FAILURES.TIMEOUT,
			"controlled timeout",
		);
	};
	await assert.rejects(
		publishAndVerifyCandidate(subject.input, subject.dependencies),
		(error) => error.code === PUBLICATION_FAILURES.TIMEOUT,
	);
	assert.equal(subject.calls.publish, 1);
	assert.equal(subject.calls.install, 0);
});
