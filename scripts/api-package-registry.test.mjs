import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import {
	RECONCILIATION_FAILURES,
	RECONCILIATION_OUTCOMES,
	RegistryReconciliationError,
	reconcileCandidate,
} from "./api-package-registry.mjs";

const sourceCommit = "0123456789abcdef0123456789abcdef01234567";

function digest(contents) {
	return `sha256:${createHash("sha256").update(contents).digest("hex")}`;
}

async function candidateFixture(t) {
	const directory = await mkdtemp(join(tmpdir(), "you-api-registry-"));
	t.after(() => rm(directory, { recursive: true, force: true }));
	const tarballPath = join(directory, "candidate.tgz");
	const contents = Buffer.from("reviewed immutable candidate");
	await writeFile(tarballPath, contents);
	return {
		contents,
		tarballPath,
		evidence: {
			packageName: "@you-agent-factory/api",
			candidateVersion: "0.0.2-dev.42.0123456789ab",
			sourceCommit,
			contractDigest: digest("contract manifest"),
			artifactDigest: digest(contents),
			inventory: ["LICENSE.md", "package.json"],
			distTag: "dev",
		},
	};
}

function registrySpy({ lookup, download }) {
	const calls = { lookup: 0, download: 0, publish: 0 };
	return {
		calls,
		async lookupVersion(input) {
			calls.lookup += 1;
			return lookup(input);
		},
		async downloadTarball(url) {
			calls.download += 1;
			return download(url);
		},
		async publish() {
			calls.publish += 1;
		},
	};
}

test("absent version returns one publish decision without mutating registry state", async (t) => {
	const candidate = await candidateFixture(t);
	const registryClient = registrySpy({
		lookup: async () => ({ status: "absent" }),
		download: async () => assert.fail("absent versions must not be downloaded"),
	});

	const result = await reconcileCandidate({ ...candidate, registryClient });

	assert.equal(result.outcome, RECONCILIATION_OUTCOMES.PUBLISH_REQUIRED);
	assert.deepEqual(registryClient.calls, { lookup: 1, download: 0, publish: 0 });
});

test("matching existing version is verified without publish or tag mutation", async (t) => {
	const candidate = await candidateFixture(t);
	const registryClient = registrySpy({
		lookup: async () => ({
			status: "present",
			tarballUrl: "https://registry.example.test/candidate.tgz",
		}),
		download: async () => candidate.contents,
	});

	const result = await reconcileCandidate({ ...candidate, registryClient });

	assert.equal(result.outcome, RECONCILIATION_OUTCOMES.VERIFIED_EXISTING);
	assert.deepEqual(registryClient.calls, { lookup: 1, download: 1, publish: 0 });
});

test("different existing tarball fails as an immutable conflict", async (t) => {
	const candidate = await candidateFixture(t);
	const registryClient = registrySpy({
		lookup: async () => ({ status: "present", tarballUrl: "controlled" }),
		download: async () => Buffer.from("different registry tarball"),
	});

	await assert.rejects(
		reconcileCandidate({ ...candidate, registryClient }),
		(error) =>
			error.code === RECONCILIATION_FAILURES.IMMUTABLE_VERSION_CONFLICT,
	);
	assert.deepEqual(registryClient.calls, { lookup: 1, download: 1, publish: 0 });
});

test("permission failure stays distinct and cannot fall through to publish", async (t) => {
	const candidate = await candidateFixture(t);
	const registryClient = registrySpy({
		lookup: async () => {
			throw new RegistryReconciliationError(
				RECONCILIATION_FAILURES.REGISTRY_PERMISSION_FAILED,
				"registry permission denied",
			);
		},
		download: async () => assert.fail("permission failures must stop lookup"),
	});

	await assert.rejects(
		reconcileCandidate({ ...candidate, registryClient }),
		(error) => error.code === RECONCILIATION_FAILURES.REGISTRY_PERMISSION_FAILED,
	);
	assert.deepEqual(registryClient.calls, { lookup: 1, download: 0, publish: 0 });
});

test("lookup, download, authentication, and candidate digest failures stay distinct", async (t) => {
	const candidate = await candidateFixture(t);
	const cases = [
		{
			expected: RECONCILIATION_FAILURES.REGISTRY_LOOKUP_FAILED,
			registryClient: registrySpy({
				lookup: async () => {
					throw new Error("controlled lookup outage");
				},
				download: async () => undefined,
			}),
		},
		{
			expected: RECONCILIATION_FAILURES.REGISTRY_DOWNLOAD_FAILED,
			registryClient: registrySpy({
				lookup: async () => ({ status: "present", tarballUrl: "controlled" }),
				download: async () => {
					throw new Error("controlled download outage");
				},
			}),
		},
		{
			expected: RECONCILIATION_FAILURES.REGISTRY_AUTHENTICATION_FAILED,
			registryClient: registrySpy({
				lookup: async () => {
					throw new RegistryReconciliationError(
						RECONCILIATION_FAILURES.REGISTRY_AUTHENTICATION_FAILED,
						"registry authentication failed",
					);
				},
				download: async () => undefined,
			}),
		},
	];

	for (const { expected, registryClient } of cases) {
		await assert.rejects(
			reconcileCandidate({ ...candidate, registryClient }),
			(error) => error.code === expected,
		);
		assert.equal(registryClient.calls.publish, 0);
	}

	await writeFile(candidate.tarballPath, "locally modified candidate");
	const unusedRegistry = registrySpy({
		lookup: async () => assert.fail("invalid candidates must fail locally"),
		download: async () => undefined,
	});
	await assert.rejects(
		reconcileCandidate({ ...candidate, registryClient: unusedRegistry }),
		(error) =>
			error.code ===
			RECONCILIATION_FAILURES.CANDIDATE_DIGEST_VERIFICATION_FAILED,
	);
	assert.deepEqual(unusedRegistry.calls, { lookup: 0, download: 0, publish: 0 });
});

test("reconciliation diagnostics expose only approved candidate fields and outcome", async (t) => {
	const candidate = await candidateFixture(t);
	const registryClient = registrySpy({
		lookup: async () => ({ status: "absent" }),
		download: async () => undefined,
	});
	const result = await reconcileCandidate({ ...candidate, registryClient });

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
