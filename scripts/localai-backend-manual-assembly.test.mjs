import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { assembleManualBackendRelease } from "./localai-backend-manual-assembly.mjs";
import { artifactArchiveName, loadConfig, minimumPublishedArchiveSizeBytes } from "./localai-backend-artifact-workflow.mjs";

const config = loadConfig();
const pinned = JSON.parse(readFileSync(new URL("../pkg/services/models/internal/artifacts/default-manifest.json", import.meta.url), "utf8"));

function metadataForCUDA() {
	const backend = config.backends.find((item) => item.id === "localai-llamacpp");
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

function fixture() {
	const root = mkdtempSync(join(tmpdir(), "localai-manual-assembly-test-"));
	const baselineDirectory = join(root, "baseline");
	mkdirSync(baselineDirectory);
	const trustedBaseline = structuredClone(pinned);
	for (const [index, entry] of trustedBaseline.artifacts.entries()) {
		const bytes = Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, index + 1);
		writeFileSync(join(baselineDirectory, entry.artifact.name), bytes);
		entry.artifact.sizeBytes = bytes.length;
		entry.artifact.sha256 = createHash("sha256").update(bytes).digest("hex");
	}
	writeFileSync(join(baselineDirectory, "manifest.json"), JSON.stringify(trustedBaseline));
	const cudaName = artifactArchiveName({
		backend: config.backends.find((item) => item.id === "localai-llamacpp"),
		target: config.targets.find((item) => item.id === "windows-amd64-cuda"),
	});
	const cudaArchive = join(root, cudaName);
	writeFileSync(cudaArchive, Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 255));
	const cudaMetadata = join(root, `${cudaName}.metadata.json`);
	writeFileSync(cudaMetadata, JSON.stringify(metadataForCUDA()));
	return { root, baselineDirectory, trustedBaseline, cudaArchive, cudaMetadata };
}

test("manual assembly retains the nine verified baseline bytes and adds Windows CUDA under a distinct tag", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	const first = assembleManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "release-one") });
	const second = assembleManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "release-two") });
	assert.equal(first.manifest.artifacts.length, 10);
	assert.equal(first.manifest.publication.releaseTag, second.manifest.publication.releaseTag);
	assert.notEqual(first.manifest.publication.releaseTag, pinned.publication.releaseTag);
	assert.equal(first.manifest.publication.packagingRevision, pinned.publication.packagingRevision);
	assert.equal(first.manifest.artifacts.at(-1).id, "localai-llamacpp/windows-amd64-cuda");
	assert.ok(readFileSync(first.notesPath, "utf8").includes("No Linux CUDA archive is included"));
	assert.deepEqual(JSON.parse(readFileSync(join(first.outputDirectory, "manifest.json"))), first.manifest);
	if (process.env.LOCALAI_MANUAL_RUN_GO_PROBE === "1") {
		execFileSync("go", ["test", "./pkg/services/models/wire", "-run", "^TestManualPublicationManifestProbe$", "-count=1"], {
			cwd: new URL("..", import.meta.url),
			env: { ...process.env, LOCALAI_MANUAL_PUBLICATION_MANIFEST: join(first.outputDirectory, "manifest.json") },
			stdio: "pipe",
		});
	}
});

test("manual assembly rejects a changed baseline archive and false CUDA provenance", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	const firstName = paths.trustedBaseline.artifacts[0].artifact.name;
	writeFileSync(join(paths.baselineDirectory, firstName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 88));
	assert.throws(() => assembleManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "bad-hash") }), /SHA-256 mismatch/);
	const original = Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 1);
	writeFileSync(join(paths.baselineDirectory, firstName), original);
	const metadata = JSON.parse(readFileSync(paths.cudaMetadata, "utf8"));
	metadata.buildInputs.packagingRevision = 2;
	writeFileSync(paths.cudaMetadata, JSON.stringify(metadata));
	assert.throws(() => assembleManualBackendRelease({ ...paths, outputDirectory: join(paths.root, "bad-metadata") }), /CUDA metadata buildInputs.packagingRevision mismatch/);
});

test("manual assembly keeps staging outside the repository", (t) => {
	const paths = fixture();
	t.after(() => rmSync(paths.root, { recursive: true, force: true }));
	assert.throws(() => assembleManualBackendRelease({
		...paths, outputDirectory: fileURLToPath(new URL("../.artifacts/manual-release", import.meta.url)),
	}), /outside the repository/);
});
