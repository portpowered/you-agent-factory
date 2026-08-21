import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	artifactArchiveName,
	createManifest,
	loadConfig,
	matrixForConfig,
	publicationIdentity,
	validateConfig,
	verifyPayload,
	verifyManifestArchives,
	writePublicationBundle,
} from "./localai-backend-artifact-workflow.mjs";

const config = loadConfig();

test("the pinned workflow config contains exactly the promised nine combinations", () => {
	const result = validateConfig(config);
	assert.deepEqual(result.errors, []);
	assert.equal(result.matrix.include.length, 9);
	assert.deepEqual(
		result.matrix.include.map(({ backend, target }) => `${backend}/${target}`),
		[
			"localai-llamacpp/darwin-arm64",
			"localai-llamacpp/linux-amd64",
			"localai-llamacpp/windows-amd64",
			"localai-whisper/darwin-arm64",
			"localai-whisper/linux-amd64",
			"localai-whisper/windows-amd64",
			"localai-vibevoice/darwin-arm64",
			"localai-vibevoice/linux-amd64",
			"localai-vibevoice/windows-amd64",
		],
	);
	assert.deepEqual(matrixForConfig(config), result.matrix);
});

test("matrix validation rejects mutable refs, floating runners, and extra targets", () => {
	const mutable = structuredClone(config);
	mutable.localaiCommit = "main";
	mutable.targets[0].runner = "macos-latest";
	mutable.targets.push({
		id: "freebsd-amd64",
		os: "freebsd",
		architecture: "amd64",
		runner: "freebsd-latest",
		buildType: "cpu",
	});
	const result = validateConfig(mutable);
	assert.ok(result.errors.some((error) => error.includes("localaiCommit")));
	assert.ok(result.errors.some((error) => error.includes("runner must not be floating")));
	assert.ok(result.errors.some((error) => error.includes("targets must be exactly")));
});

test("each target payload verifier checks the native executable identity", async (t) => {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-payload-"));
	t.after(() => rm(root, { recursive: true, force: true }));

	const windows = Buffer.alloc(80);
	windows.write("MZ", 0, "ascii");
	windows.writeUInt32LE(64, 0x3c);
	windows.write("PE\0\0", 64, "ascii");
	windows.writeUInt16LE(0x8664, 68);
	const windowsRoot = join(root, "windows");
	await writeFile(join(root, "windows-placeholder"), "placeholder");
	await mkdir(windowsRoot);
	await writeFile(join(windowsRoot, "whisper.exe"), windows);
	assert.equal(verifyPayload({ packageRoot: windowsRoot, binary: "whisper", targetId: "windows-amd64" }).bytes, 80);

	const linux = Buffer.alloc(32);
	linux[0] = 0x7f;
	linux.write("ELF", 1, "ascii");
	linux[18] = 0x3e;
	const linuxRoot = join(root, "linux");
	await mkdir(linuxRoot);
	await writeFile(join(linuxRoot, "grpc-server"), linux);
	assert.equal(verifyPayload({ packageRoot: linuxRoot, binary: "grpc-server", targetId: "linux-amd64" }).bytes, 32);

	const darwin = Buffer.alloc(32);
	darwin.writeUInt32LE(0xfeedfacf, 0);
	darwin.writeUInt32LE(0x0100000c, 4);
	const darwinRoot = join(root, "darwin");
	await mkdir(darwinRoot);
	await writeFile(join(darwinRoot, "vibevoice-cpp"), darwin);
	assert.equal(verifyPayload({ packageRoot: darwinRoot, binary: "vibevoice-cpp", targetId: "darwin-arm64" }).bytes, 32);
});

test("the validation CLI emits a matrix output suitable for GitHub Actions", async (t) => {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-matrix-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	const output = join(root, "github-output");
	const script = join(process.cwd(), "scripts", "localai-backend-artifact-workflow.mjs");
	const result = spawnSync(process.execPath, [script, "validate", "--github-output", output], { encoding: "utf8" });
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const outputText = await readFile(output, "utf8");
	assert.match(outputText, /^matrix=\{"include":\[/m);
	assert.match(outputText, /localai_commit=b224c96db6f4b87306a33a808650bfce63b12588/);
	assert.match(result.stdout, /LOCALAI_BACKEND_ARTIFACT_INPUTS_OK combinations=9/);
});

function metadataFixture(backend, target) {
	return {
		formatVersion: 1,
		backend: backend.id,
		target: {
			id: target.id,
			operatingSystem: target.os,
			architecture: target.architecture,
			buildType: target.buildType,
			accelerators: target.accelerators,
		},
		source: {
			repository: config.localaiRepository,
			commit: config.localaiCommit,
			path: backend.sourcePath,
			backendRepository: backend.sourceRepository,
			backendCommit: backend.sourceCommit,
			backendPinVariable: backend.sourcePinVariable,
		},
		protocol: { path: config.protocolPath, revision: config.protocolRevision },
		toolchain: { ...config.toolchain, grpcCommit: config.grpcCommit, vcpkgCommit: config.vcpkgCommit },
		payload: { binary: backend.binary, makeTarget: backend.makeTarget },
	};
}

async function matrixArtifactFixture(t) {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-join-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	for (const backend of config.backends) {
		for (const target of config.targets) {
			const archiveName = artifactArchiveName({ backend, target });
			await writeFile(join(root, archiveName), Buffer.from(`${backend.id}/${target.id}`));
			await writeFile(join(root, `${archiveName}.metadata.json`), `${JSON.stringify(metadataFixture(backend, target))}\n`);
		}
	}
	return root;
}

test("the join emits one P1 manifest for the exact nine archives and re-verifies their bytes", async (t) => {
	const matrixRoot = await matrixArtifactFixture(t);
	const outputRoot = await mkdtemp(join(tmpdir(), "localai-backend-release-"));
	t.after(() => rm(outputRoot, { recursive: true, force: true }));
	const result = writePublicationBundle({
		config,
		artifactDirectory: matrixRoot,
		outputDirectory: outputRoot,
		repository: "portpowered/infinite-you",
	});
	assert.equal(result.manifest.schemaVersion, 1);
	assert.equal(result.manifest.kind, "localai-backend-artifacts");
	assert.equal(result.manifest.artifacts.length, 9);
	assert.equal(result.manifest.publication.releaseTag, publicationIdentity(config).releaseTag);
	const artifact = result.manifest.artifacts.find((entry) => entry.id === "localai-llamacpp/darwin-arm64");
	assert.equal(artifact.target.operatingSystem, "darwin");
	assert.deepEqual(artifact.target.accelerators, ["metal"]);
	assert.equal(artifact.source.commit, config.localaiCommit);
	assert.equal(artifact.protocol.revision, config.protocolRevision);
	assert.match(artifact.artifact.location, new RegExp(`/${result.manifest.publication.releaseTag}/`));
	assert.match(artifact.artifact.sha256, /^[0-9a-f]{64}$/);
	assert.equal(artifact.artifact.sizeBytes, Buffer.from("localai-llamacpp/darwin-arm64").length);
	assert.deepEqual((await readdir(outputRoot)).sort(), [
		"localai-backend-localai-llamacpp-darwin-arm64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		"localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		"localai-backend-localai-llamacpp-windows-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.zip",
		"localai-backend-localai-vibevoice-darwin-arm64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		"localai-backend-localai-vibevoice-linux-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.tar.gz",
		"localai-backend-localai-vibevoice-windows-amd64-000e37282bc5bb09edc20f7047a47924122ba3a0.zip",
		"localai-backend-localai-whisper-darwin-arm64-080bbbe85230f624f0b52127f1ae1218247989f9.tar.gz",
		"localai-backend-localai-whisper-linux-amd64-080bbbe85230f624f0b52127f1ae1218247989f9.tar.gz",
		"localai-backend-localai-whisper-windows-amd64-080bbbe85230f624f0b52127f1ae1218247989f9.zip",
		"manifest.json",
	]);
});

test("the join rejects missing and unexpected matrix results", async (t) => {
	const missingRoot = await matrixArtifactFixture(t);
	const missingArchive = artifactArchiveName({ backend: config.backends[0], target: config.targets[0] });
	await rm(join(missingRoot, missingArchive));
	await assert.rejects(
		(async () => createManifest({ config, artifactDirectory: missingRoot, repository: "portpowered/infinite-you" })),
		/exactly the nine archives and nine provenance sidecars/,
	);

	const extraRoot = await matrixArtifactFixture(t);
	await writeFile(join(extraRoot, "unexpected.archive"), "unexpected");
	assert.throws(
		() => createManifest({ config, artifactDirectory: extraRoot, repository: "portpowered/infinite-you" }),
		/exactly the nine archives and nine provenance sidecars/,
	);
});

test("manifest verification rejects bytes tampered after manifest creation", async (t) => {
	const root = await matrixArtifactFixture(t);
	const manifest = createManifest({ config, artifactDirectory: root, repository: "portpowered/infinite-you" });
	const archiveName = manifest.artifacts[0].artifact.name;
	await writeFile(join(root, archiveName), "tampered bytes");
	assert.throws(
		() => verifyManifestArchives({ manifest, artifactDirectory: root }),
		/integrity mismatch/,
	);
});
