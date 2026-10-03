import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { extendManualBackendReleaseVibevoice } from "./localai-backend-manual-extend-vibevoice.mjs";
import { artifactArchiveName, loadConfig, minimumPublishedArchiveSizeBytes } from "./localai-backend-artifact-workflow.mjs";

// Extension metadata must match the current recipe publication identity.
const config = loadConfig();
const baselinePairs = [
	["localai-llamacpp", "darwin-arm64"],
	["localai-llamacpp", "linux-amd64"],
	["localai-llamacpp", "windows-amd64"],
	["localai-whisper", "darwin-arm64"],
	["localai-whisper", "linux-amd64"],
	["localai-whisper", "windows-amd64"],
	["localai-vibevoice", "darwin-arm64"],
	["localai-vibevoice", "linux-amd64"],
	["localai-vibevoice", "windows-amd64"],
	["localai-llamacpp", "windows-amd64-cuda"],
	["localai-whisper", "windows-amd64-cuda"],
];

function sha256(bytes) {
	return createHash("sha256").update(bytes).digest("hex");
}

function backendAndTarget(backendId, targetId) {
	return {
		backend: config.backends.find((item) => item.id === backendId),
		target: config.targets.find((item) => item.id === targetId),
	};
}

function metadataForVibevoiceCUDA() {
	const { backend, target } = backendAndTarget("localai-vibevoice", "windows-amd64-cuda");
	return {
		formatVersion: 1,
		backend: backend.id,
		target: { id: target.id, operatingSystem: target.os, architecture: target.architecture, buildType: target.buildType, accelerators: target.accelerators },
		source: {
			repository: config.localaiRepository, commit: config.localaiCommit, path: backend.sourcePath,
			backendRepository: backend.sourceRepository, backendCommit: backend.sourceCommit, backendPinVariable: backend.sourcePinVariable,
		},
		protocol: { path: config.protocolPath, revision: config.protocolRevision },
		payload: { binary: backend.binary, makeTarget: backend.makeTarget },
		toolchain: { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit },
		buildInputs: { nodeVersion: config.nodeVersion, packagingRevision: config.packagingRevision, actionPins: config.workflowPins, hostToolchain: config.hostToolchain },
	};
}

function fixture() {
	const root = mkdtempSync(join(tmpdir(), "localai-vibevoice-extend-test-"));
	const baselineDirectory = join(root, "baseline");
	mkdirSync(baselineDirectory);
	const tag = "localai-backends-v1-eleven-archive-fixture";
	const artifacts = baselinePairs.map(([backendId, targetId], index) => {
		const { backend, target } = backendAndTarget(backendId, targetId);
		const name = artifactArchiveName({ backend, target });
		const bytes = Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, index + 1);
		writeFileSync(join(baselineDirectory, name), bytes);
		return {
			id: `${backendId}/${targetId}`,
			backend: { id: backend.id, source: { repository: backend.sourceRepository, commit: backend.sourceCommit, pinVariable: backend.sourcePinVariable } },
			source: { repository: config.localaiRepository, commit: config.localaiCommit, path: backend.sourcePath },
			protocol: { path: config.protocolPath, revision: config.protocolRevision },
			target: { id: target.id, operatingSystem: target.os, architecture: target.architecture, accelerators: [...target.accelerators] },
			artifact: { name, location: `https://github.com/portpowered/you-agent-factory/releases/download/${tag}/${name}`, sizeBytes: bytes.length, sha256: sha256(bytes) },
		};
	});
	const manifest = {
		schemaVersion: 1,
		kind: "localai-backend-artifacts",
		publication: {
			id: tag, releaseTag: tag, repository: "portpowered/you-agent-factory", pinFingerprint: "0".repeat(64), packagingRevision: 2,
			source: { repository: config.localaiRepository, commit: config.localaiCommit },
			protocol: { path: config.protocolPath, revision: config.protocolRevision },
			toolchain: { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit },
		},
		artifacts,
	};
	const manifestBytes = `${JSON.stringify(manifest, null, 2)}\n`;
	writeFileSync(join(baselineDirectory, "manifest.json"), manifestBytes);
	const { backend, target } = backendAndTarget("localai-vibevoice", "windows-amd64-cuda");
	const cudaName = artifactArchiveName({ backend, target });
	const vibevoiceCudaArchive = join(root, cudaName);
	writeFileSync(vibevoiceCudaArchive, Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 253));
	const vibevoiceCudaMetadata = join(root, `${cudaName}.metadata.json`);
	writeFileSync(vibevoiceCudaMetadata, JSON.stringify(metadataForVibevoiceCUDA()));
	return { root, baselineDirectory, expectedBaselineManifestSha256: sha256(manifestBytes), vibevoiceCudaArchive, vibevoiceCudaMetadata, config };
}

test("VibeVoice extension stages twelve verified archives under a deterministic tag", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	const first = extendManualBackendReleaseVibevoice({ ...paths, outputDirectory: join(paths.root, "release-one") });
	const second = extendManualBackendReleaseVibevoice({ ...paths, outputDirectory: join(paths.root, "release-two") });
	assert.equal(first.manifest.artifacts.length, 12);
	assert.equal(first.manifest.artifacts.at(-1).id, "localai-vibevoice/windows-amd64-cuda");
	assert.equal(first.manifest.publication.releaseTag, second.manifest.publication.releaseTag);
	assert.match(first.manifest.publication.releaseTag, /^localai-backends-v1-[0-9a-f]{64}$/);
	assert.equal(first.manifest.publication.id, first.manifest.publication.releaseTag);
	assert.equal(first.manifest.publication.pinFingerprint, first.manifest.publication.releaseTag.slice("localai-backends-v1-".length));
	assert.equal(first.manifest.publication.packagingRevision, 2);
	assert.deepEqual(readFileSync(join(first.outputDirectory, "manifest.json")), readFileSync(join(second.outputDirectory, "manifest.json")));
	assert.deepEqual(readdirSync(first.outputDirectory).sort(), ["manifest.json", ...first.manifest.artifacts.map((entry) => entry.artifact.name)].sort());
	for (const entry of first.manifest.artifacts) {
		const bytes = readFileSync(join(first.outputDirectory, entry.artifact.name));
		assert.equal(bytes.length, entry.artifact.sizeBytes);
		assert.equal(sha256(bytes), entry.artifact.sha256);
		assert.equal(entry.artifact.location, `https://github.com/portpowered/you-agent-factory/releases/download/${first.manifest.publication.releaseTag}/${entry.artifact.name}`);
	}
	assert.deepEqual(JSON.parse(readFileSync(join(first.outputDirectory, "manifest.json"))), first.manifest);
	assert.match(readFileSync(first.notesPath, "utf8"), /localai-vibevoice\/windows-amd64-cuda/);
	writeFileSync(paths.vibevoiceCudaArchive, Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 252));
	const changed = extendManualBackendReleaseVibevoice({ ...paths, outputDirectory: join(paths.root, "release-changed") });
	assert.notEqual(changed.manifest.publication.releaseTag, first.manifest.publication.releaseTag);
	assert.notEqual(changed.manifest.artifacts.at(-1).artifact.sha256, first.manifest.artifacts.at(-1).artifact.sha256);
});

test("VibeVoice extension rejects wrong baseline pin and tampered archives", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	assert.throws(() => extendManualBackendReleaseVibevoice({ ...paths, expectedBaselineManifestSha256: "0".repeat(64), outputDirectory: join(paths.root, "bad-pin") }), /baseline manifest SHA-256 mismatch/);
	const firstName = artifactArchiveName(backendAndTarget(...baselinePairs[0]));
	writeFileSync(join(paths.baselineDirectory, firstName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 88));
	assert.throws(() => extendManualBackendReleaseVibevoice({ ...paths, outputDirectory: join(paths.root, "bad-baseline") }), /SHA-256 mismatch/);
	writeFileSync(join(paths.baselineDirectory, firstName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 1));
	writeFileSync(paths.vibevoiceCudaArchive, Buffer.alloc(minimumPublishedArchiveSizeBytes, 253));
	assert.throws(() => extendManualBackendReleaseVibevoice({ ...paths, outputDirectory: join(paths.root, "bad-cuda") }), /placeholder-sized/);
});

test("VibeVoice extension rejects false metadata and wrong configuration revision", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	const outputDirectory = join(paths.root, "bad-metadata");
	const metadata = JSON.parse(readFileSync(paths.vibevoiceCudaMetadata, "utf8"));
	metadata.buildInputs.packagingRevision = 4;
	writeFileSync(paths.vibevoiceCudaMetadata, JSON.stringify(metadata));
	assert.throws(() => extendManualBackendReleaseVibevoice({ ...paths, outputDirectory }), /vibevoice CUDA metadata buildInputs.packagingRevision mismatch/);
	assert.throws(() => extendManualBackendReleaseVibevoice({ ...paths, config: { ...config, packagingRevision: 5 }, outputDirectory: join(paths.root, "bad-config") }), /packagingRevision must be 6/);
});
