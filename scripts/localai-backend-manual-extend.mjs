import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { closeSync, copyFileSync, existsSync, lstatSync, mkdirSync, mkdtempSync, openSync, readFileSync, readSync, readdirSync, renameSync, writeFileSync } from "node:fs";
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
	artifactArchiveName, loadConfig, minimumPublishedArchiveSizeBytes,
	publicationIdentity,
} from "./localai-backend-artifact-workflow.mjs";

const repositoryRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const configPath = join(repositoryRoot, ".github", "localai-backend-artifacts.json");
const repository = "portpowered/you-agent-factory";
const whisperCudaID = "localai-whisper/windows-amd64-cuda";
const digestPattern = /^[0-9a-f]{64}$/;

// Ten-archive baseline: the nine CPU/metal archives plus the manually
// published llama Windows CUDA archive. Linux CUDA still builds natively
// and is intentionally absent.
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

function readJSON(path) {
	return JSON.parse(readFileSync(path, "utf8"));
}

function fileEvidence(path) {
	const info = lstatSync(path);
	if (!info.isFile()) throw new Error(`expected a regular file: ${path}`);
	if (info.size <= minimumPublishedArchiveSizeBytes) throw new Error(`archive is placeholder-sized: ${path}`);
	const digest = createHash("sha256");
	const buffer = Buffer.allocUnsafe(1 << 20);
	const handle = openSync(path, "r");
	let sizeBytes = 0;
	try {
		for (;;) {
			const count = readSync(handle, buffer);
			if (count === 0) break;
			digest.update(buffer.subarray(0, count));
			sizeBytes += count;
		}
	} finally {
		closeSync(handle);
	}
	if (sizeBytes !== info.size) throw new Error(`archive changed during hashing: ${path}`);
	return { sizeBytes, sha256: digest.digest("hex") };
}

function checkArchive(path, artifact) {
	const observed = fileEvidence(path);
	assert.equal(observed.sizeBytes, artifact.sizeBytes, `${artifact.name} size mismatch`);
	assert.equal(observed.sha256, artifact.sha256, `${artifact.name} SHA-256 mismatch`);
}

function checkMetadata(metadata, config, backend, target) {
	const fields = {
		formatVersion: 1,
		backend: backend.id,
		"target.id": target.id,
		"target.operatingSystem": target.os,
		"target.architecture": target.architecture,
		"target.buildType": target.buildType,
		"target.accelerators": target.accelerators,
		"source.repository": config.localaiRepository,
		"source.commit": config.localaiCommit,
		"source.path": backend.sourcePath,
		"source.backendRepository": backend.sourceRepository,
		"source.backendCommit": backend.sourceCommit,
		"source.backendPinVariable": backend.sourcePinVariable,
		"protocol.path": config.protocolPath,
		"protocol.revision": config.protocolRevision,
		"payload.binary": backend.binary,
		"payload.makeTarget": backend.makeTarget,
		"toolchain.grpcCommit": config.grpcCommit,
		"toolchain.vcpkgCommit": config.vcpkgCommit,
		"buildInputs.nodeVersion": config.nodeVersion,
		"buildInputs.packagingRevision": config.packagingRevision,
		"buildInputs.actionPins": config.workflowPins,
		"buildInputs.hostToolchain": config.hostToolchain,
	};
	for (const [key, value] of Object.entries(config.toolchain)) fields[`toolchain.${key}`] = value;
	for (const [path, expected] of Object.entries(fields)) {
		const actual = path.split(".").reduce((value, key) => value?.[key], metadata);
		assert.deepEqual(actual, expected, `whisper CUDA metadata ${path} mismatch`);
	}
}

function releaseLocation(tag, name) {
	return `https://github.com/${repository}/releases/download/${tag}/${name}`;
}

function backendAndTarget(config, backendId, targetId) {
	const backend = config.backends.find((item) => item.id === backendId);
	const target = config.targets.find((item) => item.id === targetId);
	if (!backend || !target) throw new Error(`unknown backend/target ${backendId}/${targetId}`);
	return { backend, target };
}

export function extendManualBackendRelease({
	baselineDirectory, expectedBaselineManifestSha256, whisperCudaArchive, whisperCudaMetadata, outputDirectory,
	config = loadConfig(configPath),
}) {
	if (typeof expectedBaselineManifestSha256 !== "string" || !digestPattern.test(expectedBaselineManifestSha256)) {
		throw new Error("expected baseline manifest SHA-256 must be a lowercase 64-character digest");
	}
	const baselineRoot = resolve(baselineDirectory);
	const outputRoot = resolve(outputDirectory);
	const notesPath = `${outputRoot}.release-notes.md`;
	const relativeToRepository = relative(repositoryRoot, outputRoot);
	if (relativeToRepository === "" || (!relativeToRepository.startsWith("..") && !isAbsolute(relativeToRepository))) {
		throw new Error("output directory must be outside the repository");
	}
	if (outputRoot === baselineRoot || outputRoot.startsWith(`${baselineRoot}\\`) || outputRoot.startsWith(`${baselineRoot}/`)) {
		throw new Error("output directory must be separate from baseline archives");
	}
	if (resolve(whisperCudaArchive) === outputRoot || resolve(whisperCudaMetadata) === outputRoot) {
		throw new Error("new CUDA inputs must be separate from the output directory");
	}
	if (existsSync(outputRoot)) throw new Error(`output directory must not exist: ${outputRoot}`);
	if (existsSync(notesPath)) throw new Error(`release notes path must not exist: ${notesPath}`);
	if (config.packagingRevision !== 4) {
		throw new Error(`current configuration packagingRevision must be 4, received ${config.packagingRevision}`);
	}
	const configIdentity = publicationIdentity(config);
	const manifestPath = join(baselineRoot, "manifest.json");
	const manifestBytes = readFileSync(manifestPath);
	const observedManifestSha256 = createHash("sha256").update(manifestBytes).digest("hex");
	assert.equal(observedManifestSha256, expectedBaselineManifestSha256, "baseline manifest SHA-256 mismatch");
	const baseline = JSON.parse(manifestBytes.toString("utf8"));
	assert.equal(baseline?.schemaVersion, 1, "baseline schemaVersion must be 1");
	assert.equal(baseline?.kind, "localai-backend-artifacts", "baseline kind mismatch");
	assert.equal(baseline.publication?.repository, repository, "baseline repository mismatch");
	assert.ok(typeof baseline.publication?.releaseTag === "string" && baseline.publication.releaseTag.startsWith("localai-backends-v1-"), "baseline release tag mismatch");
	assert.equal(baseline.publication?.source?.repository, config.localaiRepository, "baseline source repository mismatch");
	assert.equal(baseline.publication?.source?.commit, config.localaiCommit, "baseline source commit mismatch");
	assert.equal(baseline.publication?.protocol?.path, config.protocolPath, "baseline protocol path mismatch");
	assert.equal(baseline.publication?.protocol?.revision, config.protocolRevision, "baseline protocol revision mismatch");
	assert.deepEqual(baseline.publication?.toolchain, { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit }, "baseline toolchain mismatch");
	assert.equal(baseline.artifacts?.length, 10, "baseline must have ten entries");
	const expectedIds = baselinePairs.map(([backendId, targetId]) => `${backendId}/${targetId}`);
	assert.deepEqual(baseline.artifacts.map((entry) => entry?.id), expectedIds, "baseline artifact identities mismatch");
	const baselineNames = baseline.artifacts.map((entry) => entry.artifact.name);
	assert.deepEqual(readdirSync(baselineRoot).sort(), [...baselineNames, "manifest.json"].sort(), "baseline directory must contain exactly the ten pinned archives and manifest");
	const seenIds = new Set();
	const seenNames = new Set();
	for (const entry of baseline.artifacts) {
		if (seenIds.has(entry.id)) throw new Error(`baseline contains duplicate artifact identity ${entry.id}`);
		if (seenNames.has(entry.artifact?.name)) throw new Error(`baseline contains duplicate archive name ${entry.artifact?.name}`);
		seenIds.add(entry.id);
		seenNames.add(entry.artifact?.name);
		const [backendId, targetId] = entry.id.split("/");
		const { backend, target } = backendAndTarget(config, backendId, targetId);
		assert.equal(entry.artifact?.name, artifactArchiveName({ backend, target }), `${entry.id} archive name mismatch`);
		assert.equal(entry.artifact?.location, releaseLocation(baseline.publication.releaseTag, entry.artifact.name), `${entry.id} location mismatch`);
		assert.equal(entry.backend?.id, backendId, `${entry.id} backend mismatch`);
		assert.equal(entry.source?.repository, config.localaiRepository, `${entry.id} source repository mismatch`);
		assert.equal(entry.source?.commit, config.localaiCommit, `${entry.id} source commit mismatch`);
		assert.equal(entry.protocol?.revision, config.protocolRevision, `${entry.id} protocol revision mismatch`);
		checkArchive(join(baselineRoot, entry.artifact.name), entry.artifact);
	}
	const { backend, target } = backendAndTarget(config, "localai-whisper", "windows-amd64-cuda");
	const cudaName = artifactArchiveName({ backend, target });
	if (seenIds.has(whisperCudaID)) throw new Error(`baseline already contains ${whisperCudaID}`);
	if (seenNames.has(cudaName)) throw new Error(`new archive name overwrites a baseline archive: ${cudaName}`);
	assert.equal(basename(whisperCudaArchive), cudaName, "whisper CUDA archive name mismatch");
	const metadata = readJSON(whisperCudaMetadata);
	checkMetadata(metadata, config, backend, target);
	const cudaEvidence = fileEvidence(whisperCudaArchive);
	const fingerprint = createHash("sha256").update(JSON.stringify({
		marker: "manual-whisper-windows-cuda-extension-v1",
		baselineTag: baseline.publication.releaseTag,
		baselineManifestSha256: expectedBaselineManifestSha256,
		baselineArchives: baseline.artifacts.map((entry) => ({ id: entry.id, name: entry.artifact.name, sha256: entry.artifact.sha256 })),
		whisperCUDA: { name: cudaName, sha256: cudaEvidence.sha256 },
		configPackagingRevision: config.packagingRevision,
		configPinFingerprint: configIdentity.pinFingerprint,
		source: { repository: config.localaiRepository, commit: config.localaiCommit },
		protocol: { path: config.protocolPath, revision: config.protocolRevision },
	})).digest("hex");
	const releaseTag = `localai-backends-v1-${fingerprint}`;
	const whisperEntry = {
		id: whisperCudaID,
		backend: { id: backend.id, source: { repository: backend.sourceRepository, commit: backend.sourceCommit, pinVariable: backend.sourcePinVariable } },
		source: { repository: config.localaiRepository, commit: config.localaiCommit, path: backend.sourcePath },
		protocol: { path: config.protocolPath, revision: config.protocolRevision },
		target: { id: target.id, operatingSystem: target.os, architecture: target.architecture, accelerators: [...target.accelerators] },
		artifact: { name: cudaName, location: releaseLocation(releaseTag, cudaName), ...cudaEvidence },
	};
	const artifacts = [
		...baseline.artifacts.map((old) => ({
			...old, artifact: { ...old.artifact, location: releaseLocation(releaseTag, old.artifact.name) },
		})),
		whisperEntry,
	];
	assert.equal(artifacts.length, 11, "new publication must have eleven entries");
	const manifest = {
		...baseline,
		publication: {
			...baseline.publication,
			id: releaseTag, releaseTag,
			pinFingerprint: fingerprint,
		},
		artifacts,
	};
	mkdirSync(dirname(outputRoot), { recursive: true });
	const stage = mkdtempSync(join(dirname(outputRoot), ".localai-manual-extend-stage-"));
	for (const entry of artifacts) {
		const source = entry.id === whisperCudaID ? whisperCudaArchive : join(baselineRoot, entry.artifact.name);
		copyFileSync(source, join(stage, entry.artifact.name));
	}
	writeFileSync(join(stage, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
	for (const entry of artifacts) checkArchive(join(stage, entry.artifact.name), entry.artifact);
	assert.deepEqual(readdirSync(stage).sort(), [...artifacts.map((entry) => entry.artifact.name), "manifest.json"].sort());
	renameSync(stage, outputRoot);
	writeFileSync(notesPath, `# Pinned LocalAI backends ${releaseTag}\n\nThis manual extension contains eleven backend archives. Ten archives are byte-for-byte copies from ${baseline.publication.releaseTag} (manifest SHA-256 ${expectedBaselineManifestSha256}) and retain their original packaging revision ${baseline.publication.packagingRevision} provenance. The new localai-whisper/windows-amd64-cuda archive was built with configuration packaging revision ${config.packagingRevision}. The manifest retains packagingRevision ${baseline.publication.packagingRevision} for the reused baseline; its distinct pin fingerprint includes all eleven archive hashes and the current configuration fingerprint. No Linux CUDA archive is included.\n`);
	return { outputDirectory: outputRoot, notesPath, manifest };
}

function option(args, key) {
	const index = args.indexOf(key);
	if (index < 0 || !args[index + 1]) throw new Error(`${key} is required`);
	return args[index + 1];
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	try {
		const args = process.argv.slice(2);
		const result = extendManualBackendRelease({
			baselineDirectory: option(args, "--baseline-directory"),
			expectedBaselineManifestSha256: option(args, "--expected-baseline-manifest-sha256"),
			whisperCudaArchive: option(args, "--whisper-cuda-archive"),
			whisperCudaMetadata: option(args, "--whisper-cuda-metadata"),
			outputDirectory: option(args, "--output-directory"),
		});
		process.stdout.write(`LOCALAI_MANUAL_EXTEND_BUNDLE_OK tag=${result.manifest.publication.releaseTag} archives=${result.manifest.artifacts.length} directory=${result.outputDirectory} notes=${result.notesPath}\n`);
	} catch (error) {
		console.error(`localai-backend-manual-extend: ${error.message}`);
		process.exitCode = 1;
	}
}
