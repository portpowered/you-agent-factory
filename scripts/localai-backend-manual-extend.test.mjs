import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { extendManualBackendRelease } from "./localai-backend-manual-extend.mjs";
import { artifactArchiveName, loadConfig, minimumPublishedArchiveSizeBytes } from "./localai-backend-artifact-workflow.mjs";

const config = loadConfig();
assert.equal(config.packagingRevision, 6);

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
];

function metadataForWhisperCUDA() {
	const backend = config.backends.find((item) => item.id === "localai-whisper");
	const target = config.targets.find((item) => item.id === "windows-amd64-cuda");
	return {
		formatVersion: 1, backend: backend.id,
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

function baselineManifest(tag) {
	const artifacts = baselinePairs.map(([backendId, targetId], index) => {
		const backend = config.backends.find((item) => item.id === backendId);
		const target = config.targets.find((item) => item.id === targetId);
		const name = artifactArchiveName({ backend, target });
		return {
			id: `${backendId}/${targetId}`,
			backend: { id: backend.id, source: { repository: backend.sourceRepository, commit: backend.sourceCommit, pinVariable: backend.sourcePinVariable } },
			source: { repository: config.localaiRepository, commit: config.localaiCommit, path: backend.sourcePath },
			protocol: { path: config.protocolPath, revision: config.protocolRevision },
			target: { id: target.id, operatingSystem: target.os, architecture: target.architecture, accelerators: [...target.accelerators] },
			artifact: { name, location: `https://github.com/portpowered/you-agent-factory/releases/download/${tag}/${name}`, sizeBytes: 0, sha256: "0".repeat(64), __fill: index + 1 },
		};
	});
	return {
		schemaVersion: 1,
		kind: "localai-backend-artifacts",
		publication: {
			id: tag,
			releaseTag: tag,
			repository: "portpowered/you-agent-factory",
			pinFingerprint: "0".repeat(64),
			packagingRevision: 2,
			source: { repository: config.localaiRepository, commit: config.localaiCommit },
			protocol: { path: config.protocolPath, revision: config.protocolRevision },
			toolchain: { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit },
		},
		artifacts,
	};
}

function fixture() {
	const root = mkdtempSync(join(tmpdir(), "localai-manual-extend-test-"));
	const baselineDirectory = join(root, "baseline");
	mkdirSync(baselineDirectory);
	const tag = "localai-backends-v1-ten-archive-fixture";
	const manifest = baselineManifest(tag);
	for (const entry of manifest.artifacts) {
		const bytes = Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, entry.artifact.__fill);
		writeFileSync(join(baselineDirectory, entry.artifact.name), bytes);
		delete entry.artifact.__fill;
		entry.artifact.sizeBytes = bytes.length;
		entry.artifact.sha256 = createHash("sha256").update(bytes).digest("hex");
	}
	const manifestBytes = `${JSON.stringify(manifest, null, 2)}\n`;
	writeFileSync(join(baselineDirectory, "manifest.json"), manifestBytes);
	const expectedBaselineManifestSha256 = createHash("sha256").update(manifestBytes).digest("hex");
	const backend = config.backends.find((item) => item.id === "localai-whisper");
	const target = config.targets.find((item) => item.id === "windows-amd64-cuda");
	const cudaName = artifactArchiveName({ backend, target });
	const whisperCudaArchive = join(root, cudaName);
	writeFileSync(whisperCudaArchive, Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 253));
	const whisperCudaMetadata = join(root, `${cudaName}.metadata.json`);
	writeFileSync(whisperCudaMetadata, JSON.stringify(metadataForWhisperCUDA()));
	return { root, baselineDirectory, expectedBaselineManifestSha256, whisperCudaArchive, whisperCudaMetadata };
}

test("manual extension adds whisper Windows CUDA under a deterministic new tag", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	const first = extendManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "release-one") });
	const second = extendManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "release-two") });
	assert.equal(first.manifest.artifacts.length, 11);
	assert.equal(first.manifest.artifacts.at(-1).id, "localai-whisper/windows-amd64-cuda");
	assert.equal(first.manifest.publication.releaseTag, second.manifest.publication.releaseTag);
	assert.notEqual(first.manifest.publication.releaseTag, "localai-backends-v1-ten-archive-fixture");
	assert.equal(first.manifest.publication.packagingRevision, 2);
	for (const entry of first.manifest.artifacts) {
		assert.ok(entry.artifact.location.includes(first.manifest.publication.releaseTag));
	}
	assert.ok(readFileSync(first.notesPath, "utf8").includes("localai-whisper/windows-amd64-cuda"));
	assert.ok(readFileSync(first.notesPath, "utf8").includes("No Linux CUDA archive is included"));
	assert.deepEqual(JSON.parse(readFileSync(join(first.outputDirectory, "manifest.json"))), first.manifest);
});

test("manual extension rejects a wrong manifest pin, tampered bytes, and false provenance", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	assert.throws(() => extendManualBackendRelease({ ...paths, expectedBaselineManifestSha256: "0".repeat(64), outputDirectory: join(paths.root, "bad-pin") }), /baseline manifest SHA-256 mismatch/);
	const firstName = baselinePairs.slice(0, 1).map(([backendId, targetId]) => artifactArchiveName({
		backend: config.backends.find((item) => item.id === backendId),
		target: config.targets.find((item) => item.id === targetId),
	}))[0];
	writeFileSync(join(paths.baselineDirectory, firstName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 88));
	assert.throws(() => extendManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "bad-hash") }), /SHA-256 mismatch/);
	writeFileSync(join(paths.baselineDirectory, firstName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 1));
	const metadata = JSON.parse(readFileSync(paths.whisperCudaMetadata, "utf8"));
	metadata.buildInputs.packagingRevision = 2;
	writeFileSync(paths.whisperCudaMetadata, JSON.stringify(metadata));
	assert.throws(() => extendManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "bad-metadata") }), /whisper CUDA metadata buildInputs.packagingRevision mismatch/);
});

test("manual extension keeps staging outside the repository", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	assert.throws(() => extendManualBackendRelease({
		...paths, outputDirectory: fileURLToPath(new URL("../.artifacts/manual-extend", import.meta.url)),
	}), /outside the repository/);
});
