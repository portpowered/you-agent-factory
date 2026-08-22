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
	linux[4] = 2;
	linux[5] = 1;
	linux[18] = 0x3e;
	const linuxRoot = join(root, "linux");
	await mkdir(linuxRoot);
	await writeFile(join(linuxRoot, "llama-cpp-cpu-all"), linux);
	assert.equal(verifyPayload({ packageRoot: linuxRoot, binary: "llama-cpp-cpu-all", targetId: "linux-amd64" }).bytes, 32);

	const darwin = Buffer.alloc(32);
	darwin.writeUInt32LE(0xfeedfacf, 0);
	darwin.writeUInt32LE(0x0100000c, 4);
	const darwinRoot = join(root, "darwin");
	await mkdir(darwinRoot);
	await writeFile(join(darwinRoot, "vibevoice-cpp"), darwin);
	assert.equal(verifyPayload({ packageRoot: darwinRoot, binary: "vibevoice-cpp", targetId: "darwin-arm64" }).bytes, 32);

	const universalRoot = join(root, "darwin-universal");
	await mkdir(universalRoot);
	const universal = Buffer.alloc(8 + 2 * 20 + 1);
	universal.writeUInt32BE(0xcafebabe, 0);
	universal.writeUInt32BE(2, 4);
	universal.writeUInt32BE(0x01000007, 8);
	universal.writeUInt32BE(0x0100000c, 28);
	universal.writeUInt32BE(48, 36);
	universal.writeUInt32BE(1, 40);
	await writeFile(join(universalRoot, "vibevoice-cpp"), universal);
	assert.equal(verifyPayload({ packageRoot: universalRoot, binary: "vibevoice-cpp", targetId: "darwin-arm64" }).bytes, universal.length);

	const x86UniversalRoot = join(root, "darwin-x86-universal");
	await mkdir(x86UniversalRoot);
	const x86Universal = Buffer.alloc(8 + 20 + 1);
	x86Universal.writeUInt32BE(0xcafebabe, 0);
	x86Universal.writeUInt32BE(1, 4);
	x86Universal.writeUInt32BE(0x01000007, 8);
	x86Universal.writeUInt32BE(28, 16);
	x86Universal.writeUInt32BE(1, 20);
	await writeFile(join(x86UniversalRoot, "vibevoice-cpp"), x86Universal);
	assert.throws(
		() => verifyPayload({ packageRoot: x86UniversalRoot, binary: "vibevoice-cpp", targetId: "darwin-arm64" }),
		/arm64 Darwin Mach-O/,
	);
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
	assert.match(outputText, /protobuf_version=24\.3/);
	assert.match(outputText, /windows_msys_packages=make=4\.4\.1-3/);
	assert.match(outputText, /windows_vcpkg_triplet=x64-mingw-static-release/);
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
		buildInputs: { nodeVersion: config.nodeVersion, actionPins: config.workflowPins, hostToolchain: config.hostToolchain },
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

test("the backend build uses one mutually-exclusive literal shell per runner OS", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	const buildSteps = [...workflow.matchAll(/      - name: Build the backend package on (Windows|Unix)\r?\n[\s\S]*?(?=\r?\n      - name:)/g)];

	assert.deepEqual(
		buildSteps.map(([, platform]) => platform),
		["Windows", "Unix"],
	);
	const windowsStep = buildSteps.find(([, platform]) => platform === "Windows")[0];
	const unixStep = buildSteps.find(([, platform]) => platform === "Unix")[0];
	assert.match(windowsStep, /if: runner\.os == 'Windows'/);
	assert.match(windowsStep, /shell: msys2 \{0\}/);
	assert.equal((windowsStep.match(/bash scripts\/build-localai-backend-artifact\.sh/g) ?? []).length, 1);
	assert.match(unixStep, /if: runner\.os != 'Windows'/);
	assert.match(unixStep, /shell: bash/);
	assert.equal((unixStep.match(/bash scripts\/build-localai-backend-artifact\.sh/g) ?? []).length, 1);
	assert.doesNotMatch(workflow, /shell:\s*\$\{\{\s*runner\.os/);
});

test("the workflow uses immutable actions, package inputs, and the pinned tag guard", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	assert.match(workflow, /timeout-minutes: 180/);
	for (const revision of Object.values(config.workflowPins)) assert.match(workflow, new RegExp(`@${revision}`));
	assert.doesNotMatch(workflow, /uses:\s+[^\n]+@(v\d|main|master|latest)\b/);
	assert.doesNotMatch(workflow, /update:\s*true/);
	assert.match(workflow, /curl --fail --silent --show-error --location "\$formula" --output "\$formula_path"/);
	assert.match(workflow, /brew tap-new --no-git "\$tap"/);
	assert.match(workflow, /tap_path="\$\(brew --repository "\$tap"\)"/);
	assert.match(workflow, /cp "\$formula_path" "\$tap_path\/Formula\/make\.rb"/);
	assert.match(workflow, /brew install --formula "\$tap\/make"/);
	assert.match(workflow, /pacman -Sy --noconfirm/);
	assert.match(workflow, /pacman -S --needed --noconfirm --overwrite '\*'/);
	assert.doesNotMatch(workflow, /apt-get/);
	assert.doesNotMatch(workflow, /brew install make\b/);
	assert.match(workflow, /windows_vcpkg_triplet/);
	assert.match(
		workflow,
		/vcpkg\.exe install grpc:\$\{\{ needs\.validate-inputs\.outputs\.windows_vcpkg_triplet \}\}[^\n]*--overlay-triplets=\$overlayTriplets/,
	);
	assert.match(workflow, /windows_vcpkg_triplet/);
	assert.match(workflow, /git\/ref\/tags/);
	assert.match(workflow, /git\/tags/);
	assert.match(workflow, /exists but its target could not be resolved/);
	assert.equal(
		config.backends.find(({ id }) => id === "localai-llamacpp").binary,
		"llama-cpp-cpu-all",
	);
	const buildScript = await readFile("scripts/build-localai-backend-artifact.sh", "utf8");
	assert.match(buildScript, /backend\/cpp\/grpc/);
	assert.match(buildScript, /compatibility_path="\$\{LOCALAI_ROOT\}\/backend\/grpc"/);
	assert.match(buildScript, /VCPKG_OVERLAY_TRIPLETS/);
	assert.match(buildScript, /grpc-server/);
	assert.match(buildScript, /llama-cpp-cpu-all/);
});

test("the pinned build repairs LocalAI path and Darwin shell incompatibilities", async () => {
	const buildScript = await readFile("scripts/build-localai-backend-artifact.sh", "utf8");
	assert.match(buildScript, /ensure_localai_grpc_compat_path/);
	assert.match(buildScript, /ln -s "\$grpc_path" "\$compatibility_path"/);
	assert.match(buildScript, /WINDOWS_NODE_DIR/);
	assert.match(buildScript, /node_bin_dir="\$\(cygpath -u "\$WINDOWS_NODE_DIR"\)"/);
	assert.match(buildScript, /export PATH="\$node_bin_dir:\$PATH"/);
	assert.match(buildScript, /WINDOWS_GO_DIR/);
	assert.match(buildScript, /go_bin_dir="\$\(cygpath -u "\$WINDOWS_GO_DIR"\)"/);
	assert.match(buildScript, /export PATH="\$go_bin_dir:\$PATH"/);
	assert.match(buildScript, /WINDOWS_CMAKE_DIR/);
	assert.match(buildScript, /cmake_bin_dir="\$\(cygpath -u "\$WINDOWS_CMAKE_DIR"\)"/);
	assert.match(buildScript, /export PATH="\$cmake_bin_dir:\$PATH"/);
	assert.match(buildScript, /stage_darwin_go_package/);
	assert.match(buildScript, /for library in "\$\{backend_path\}"\/libgo\*\.dylib "\$\{backend_path\}"\/libgo\*\.so/);
	assert.match(buildScript, /BUILD_TYPE="\$BUILD_TYPE" JOBS=2 "\$binary"/);
	assert.match(buildScript, /localai-whisper\|localai-vibevoice\)\r?\n\s+if \[\[ "\$TARGET_ID" == "darwin-arm64" \]\]/);
});

test("the Windows workflow exports setup-node's directory to the MSYS2 build shell", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	assert.match(workflow, /- name: Expose the pinned Node tool to MSYS2/);
	assert.match(workflow, /if: runner\.os == 'Windows'/);
	assert.match(workflow, /Get-Command node -CommandType Application -ErrorAction Stop/);
	assert.match(workflow, /Select-Object -First 1/);
	assert.match(workflow, /WINDOWS_NODE_DIR=\$\(\$nodeCommand\.Source \| Split-Path -Parent\)/);
});

test("the Windows workflow exports setup-go's directory to the MSYS2 build shell", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	assert.match(workflow, /- name: Expose the pinned Go tool to MSYS2/);
	assert.match(workflow, /if: runner\.os == 'Windows'/);
	assert.match(workflow, /Get-Command go -CommandType Application -ErrorAction Stop/);
	assert.match(workflow, /Select-Object -First 1/);
	assert.match(workflow, /WINDOWS_GO_DIR=\$\(\$goCommand\.Source \| Split-Path -Parent\)/);
});

test("the Windows workflow exports setup-cmake's directory to the MSYS2 build shell", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	assert.match(workflow, /- name: Expose the pinned CMake tool to MSYS2/);
	assert.match(workflow, /if: runner\.os == 'Windows'/);
	assert.match(workflow, /Get-Command cmake -CommandType Application -ErrorAction Stop/);
	assert.match(workflow, /Select-Object -First 1/);
	assert.match(workflow, /WINDOWS_CMAKE_DIR=\$\(\$cmakeCommand\.Source \| Split-Path -Parent\)/);
});

test("the pinned llama build preserves recursive protobuf arguments and Darwin compatibility", async () => {
	const buildScript = await readFile("scripts/build-localai-backend-artifact.sh", "utf8");
	assert.match(buildScript, /run_direct_grpc_server_make\(\)/);
	assert.match(buildScript, /_PROTOBUF_PROTOC=\$\{grpc_path\}\/installed_packages\/bin\/proto/);
	assert.match(buildScript, /_GRPC_CPP_PLUGIN_EXECUTABLE=\$\{grpc_path\}\/installed_packages\/bin\/grpc_cpp_plugin/);
	assert.match(buildScript, /PATH=\$\{grpc_path\}\/installed_packages\/bin:\$\{PATH\}/);
	assert.match(buildScript, /run_direct_grpc_server_make "\$\{cmake_args_text\} \$\{grpc_added_cmake_args\}" BUILD_TYPE=cpu BUILD_GRPC_FOR_BACKEND_LLAMA=1 grpc-server/);
	assert.match(buildScript, /patch_llama_grpc_source\(\)/);
	assert.match(buildScript, /llama\.cpp\/tools\/grpc-server/);
	assert.match(buildScript, /const needle = "reply->set_message\(arr\);"/);
	assert.match(buildScript, /const replacement = "reply->set_message\(arr\.dump\(\)\);"/);
	assert.match(buildScript, /-DProtobuf_PROTOC_EXECUTABLE=\$\{protoc_path\}/);
	assert.match(buildScript, /darwin_llama_cmake_args="\$\{cmake_args_text\} \$\{grpc_added_cmake_args\} -DGGML_CPU_ARM_ARCH=armv8\.2-a\+dotprod"/);
	assert.match(buildScript, /run_direct_grpc_server_make "\$darwin_llama_cmake_args" "\$\{os_make_args\[@\]\}" BUILD_TYPE="\$BUILD_TYPE" BUILD_GRPC_FOR_BACKEND_LLAMA=1 grpc-server/);
	assert.match(buildScript, /cp -f "\$\{backend_path\}\/grpc-server" "\$\{backend_path\}\/llama-cpp-cpu-all"/);
	assert.match(buildScript, /CMAKE_ARGS="\$cmake_args_text" "\$make_command" -C "\$backend_path" "\$\{os_make_args\[@\]\}" BUILD_TYPE="\$BUILD_TYPE" BUILD_GRPC_FOR_BACKEND_LLAMA=1 llama-cpp-cpu-all/);
	assert.doesNotMatch(buildScript, /BUILD_GRPC_FOR_BACKEND_LLAMA=1 CMAKE_ARGS="\$cmake_args_text"/);
	assert.doesNotMatch(buildScript, /llama_make_args=/);
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
