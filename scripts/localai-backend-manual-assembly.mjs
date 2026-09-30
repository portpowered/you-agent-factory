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
const pinnedBaselinePath = join(repositoryRoot, "pkg", "services", "models", "internal", "artifacts", "default-manifest.json");
const repository = "portpowered/you-agent-factory";
const cudaID = "localai-llamacpp/windows-amd64-cuda";

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
		assert.deepEqual(actual, expected, `CUDA metadata ${path} mismatch`);
	}
}

function releaseLocation(tag, name) {
	return `https://github.com/${repository}/releases/download/${tag}/${name}`;
}

export function assembleManualBackendRelease({
	baselineDirectory, cudaArchive, cudaMetadata, outputDirectory,
	config = loadConfig(configPath), trustedBaseline = readJSON(pinnedBaselinePath),
}) {
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
	if (existsSync(outputRoot)) throw new Error(`output directory must not exist: ${outputRoot}`);
	if (existsSync(notesPath)) throw new Error(`release notes path must not exist: ${notesPath}`);
	const baseline = readJSON(join(baselineRoot, "manifest.json"));
	assert.deepEqual(baseline, trustedBaseline, "baseline manifest differs from pinned trusted manifest");
	assert.equal(baseline.publication.repository, repository);
	assert.equal(baseline.artifacts.length, 9, "baseline must have nine entries");
	const configIdentity = publicationIdentity(config);
	const backend = config.backends.find((item) => item.id === "localai-llamacpp");
	const target = config.targets.find((item) => item.id === "windows-amd64-cuda");
	if (!backend || !target || config.packagingRevision <= baseline.publication.packagingRevision) {
		throw new Error("CUDA target and a newer packaging revision are required");
	}
	assert.equal(baseline.publication.source.repository, config.localaiRepository);
	assert.equal(baseline.publication.source.commit, config.localaiCommit);
	assert.equal(baseline.publication.protocol.path, config.protocolPath);
	assert.equal(baseline.publication.protocol.revision, config.protocolRevision);
	assert.deepEqual(baseline.publication.toolchain, { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit });
	const baselineNames = baseline.artifacts.map((entry) => entry.artifact.name);
	assert.deepEqual(readdirSync(baselineRoot).sort(), [...baselineNames, "manifest.json"].sort(), "baseline directory must contain exactly the pinned nine archives and manifest");
	const baselineByID = new Map();
	for (const entry of baseline.artifacts) {
		const expectedBackend = config.backends.find((item) => item.id === entry.backend.id);
		const expectedTarget = config.targets.find((item) => item.id === entry.target.id);
		if (!expectedBackend || !expectedTarget || entry.id === cudaID) throw new Error(`unexpected baseline entry ${entry.id}`);
		assert.equal(entry.id, `${expectedBackend.id}/${expectedTarget.id}`);
		assert.equal(entry.artifact.name, artifactArchiveName({ backend: expectedBackend, target: expectedTarget }));
		assert.equal(entry.artifact.location, releaseLocation(baseline.publication.releaseTag, entry.artifact.name));
		checkArchive(join(baselineRoot, entry.artifact.name), entry.artifact);
		baselineByID.set(entry.id, entry);
	}
	assert.equal(baselineByID.size, 9, "baseline identities must be unique");
	const cudaName = artifactArchiveName({ backend, target });
	assert.equal(basename(cudaArchive), cudaName, "CUDA archive name mismatch");
	const metadata = readJSON(cudaMetadata);
	checkMetadata(metadata, config, backend, target);
	const cudaEvidence = fileEvidence(cudaArchive);
	const fingerprint = createHash("sha256").update(JSON.stringify({
		marker: "manual-windows-cuda-publication-v1",
		baselineTag: baseline.publication.releaseTag,
		baselineArchives: baseline.artifacts.map((entry) => ({ id: entry.id, name: entry.artifact.name, sha256: entry.artifact.sha256 })),
		windowsCUDA: { name: cudaName, sha256: cudaEvidence.sha256 },
		configPackagingRevision: config.packagingRevision,
		configPinFingerprint: configIdentity.pinFingerprint,
		source: { repository: config.localaiRepository, commit: config.localaiCommit },
		protocol: { path: config.protocolPath, revision: config.protocolRevision },
	})).digest("hex");
	const releaseTag = `localai-backends-v1-${fingerprint}`;
	const cudaEntry = {
		id: cudaID,
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
		cudaEntry,
	];
	assert.equal(artifacts.length, 10, "new publication must have ten entries");
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
	const stage = mkdtempSync(join(dirname(outputRoot), ".localai-manual-stage-"));
	for (const entry of artifacts) {
		const source = entry.id === cudaID ? cudaArchive : join(baselineRoot, entry.artifact.name);
		copyFileSync(source, join(stage, entry.artifact.name));
	}
	writeFileSync(join(stage, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
	for (const entry of artifacts) checkArchive(join(stage, entry.artifact.name), entry.artifact);
	assert.deepEqual(readdirSync(stage).sort(), [...artifacts.map((entry) => entry.artifact.name), "manifest.json"].sort());
	renameSync(stage, outputRoot);
	writeFileSync(notesPath, `# Pinned LocalAI backends ${releaseTag}\n\nThis manual publication contains ten backend archives. Nine archives are byte-for-byte copies from ${baseline.publication.releaseTag} and retain their original packaging revision ${baseline.publication.packagingRevision} provenance. The new localai-llamacpp/windows-amd64-cuda archive was built with configuration packaging revision ${config.packagingRevision}. The manifest retains packagingRevision ${baseline.publication.packagingRevision} for the reused baseline; its distinct pin fingerprint includes all ten archive hashes and the current configuration fingerprint. No Linux CUDA archive is included.\n`);
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
		const result = assembleManualBackendRelease({
			baselineDirectory: option(args, "--baseline-directory"),
			cudaArchive: option(args, "--cuda-archive"),
			cudaMetadata: option(args, "--cuda-metadata"),
			outputDirectory: option(args, "--output-directory"),
		});
		process.stdout.write(`LOCALAI_MANUAL_BACKEND_BUNDLE_OK tag=${result.manifest.publication.releaseTag} archives=${result.manifest.artifacts.length} directory=${result.outputDirectory} notes=${result.notesPath}\n`);
	} catch (error) {
		console.error(`localai-backend-manual-assembly: ${error.message}`);
		process.exitCode = 1;
	}
}
