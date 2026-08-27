import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	artifactArchiveName,
	createManifest,
	loadConfig,
	matrixForConfig,
	minimumPublishedArchiveSizeBytes,
	publicationIdentity,
	validateConfig,
	verifyPayload,
	verifyManifestArchives,
	writePublicationBundle,
} from "./localai-backend-artifact-workflow.mjs";
import { patchWindowsGoLoader } from "./localai-backend-windows-patch.mjs";

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

test("publication identity requires an explicit packaging revision", () => {
	const invalid = structuredClone(config);
	delete invalid.packagingRevision;
	assert.throws(() => publicationIdentity(invalid), /packagingRevision must be a positive safe integer/);
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
	assert.match(outputText, /mingw-w64-x86_64-make=4\.4\.1-5/);
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
		buildInputs: { nodeVersion: config.nodeVersion, packagingRevision: config.packagingRevision, actionPins: config.workflowPins, hostToolchain: config.hostToolchain },
		payload: { binary: backend.binary, makeTarget: backend.makeTarget },
	};
}

async function matrixArtifactFixture(t) {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-join-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	for (const backend of config.backends) {
		for (const target of config.targets) {
			const archiveName = artifactArchiveName({ backend, target });
			await writeFile(join(root, archiveName), archiveFixtureBytes(backend, target));
			await writeFile(join(root, `${archiveName}.metadata.json`), `${JSON.stringify(metadataFixture(backend, target))}\n`);
		}
	}
	return root;
}

function archiveFixtureBytes(backend, target) {
	const bytes = Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 0x42);
	bytes.write(`${backend.id}/${target.id}`);
	return bytes;
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
	assert.equal(result.manifest.publication.packagingRevision, config.packagingRevision);
	const artifact = result.manifest.artifacts.find((entry) => entry.id === "localai-llamacpp/darwin-arm64");
	assert.equal(artifact.target.operatingSystem, "darwin");
	assert.deepEqual(artifact.target.accelerators, ["metal"]);
	assert.equal(artifact.source.commit, config.localaiCommit);
	assert.equal(artifact.protocol.revision, config.protocolRevision);
	assert.match(artifact.artifact.location, new RegExp(`/${result.manifest.publication.releaseTag}/`));
	assert.match(artifact.artifact.sha256, /^[0-9a-f]{64}$/);
	assert.equal(artifact.artifact.sizeBytes, minimumPublishedArchiveSizeBytes + 1);
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

function buildPlan({ backend, target, buildType, environment = {} }) {
	const script = "scripts/build-localai-backend-artifact.sh";
	const exports = [
		["LOCALAI_ROOT", "/tmp/localai-plan-fixture"],
		["BACKEND_ID", backend],
		["TARGET_ID", target],
		["BUILD_TYPE", buildType],
		["GRPC_COMMIT", "0000000000000000000000000000000000000000"],
		["BACKEND_SOURCE_COMMIT", "1111111111111111111111111111111111111111"],
		["PROTOBUF_VERSION", "24.3"],
		["GRPC_VERSION", "1.68.1"],
		["LOCALAI_BUILD_PLAN_ONLY", "1"],
	];
	const command = `export ${exports.map(([name, value]) => `${name}=${value}`).join(" ")}; bash ${script}`;
	const result = spawnSync("bash", ["-c", command], {
		encoding: "utf8",
		env: { ...process.env, ...environment },
		windowsHide: true,
	});
	if (result.error?.code === "ENOENT") return null;
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const line = result.stdout.trim();
	assert.match(line, /^LOCALAI_BACKEND_BUILD_PLAN /);
	return Object.fromEntries(line.replace(/^LOCALAI_BACKEND_BUILD_PLAN /, "").split(" ").map((entry) => entry.split("=")));
}

test("the build harness selects the Windows and Unix strategies at runtime", (t) => {
	const probe = spawnSync("bash", ["--version"], { encoding: "utf8", windowsHide: true });
	if (probe.error?.code === "ENOENT") {
		t.skip("bash is required for the executable build harness");
		return;
	}

	assert.deepEqual(buildPlan({ backend: "localai-llamacpp", target: "windows-amd64", buildType: "cpu" }), {
		backend: "localai-llamacpp",
		target: "windows-amd64",
		shell: "msys2",
		strategy: "windows-llamacpp-grpc",
		binary: "llama-cpp-cpu-all",
		cxx_standard: "17",
		cmake_generator: "mingw-makefiles",
		cmake_make_program: "mingw32-make",
		windows_minimum_target: "0x0602",
		grpc_protobuf_source: "pinned",
		grpc_executable_suffix: ".exe",
		go_dynamic_loader: "none",
		windows_library_name: "none",
		grpc_dependency_mode: "standalone",
	});
	assert.deepEqual(buildPlan({ backend: "localai-llamacpp", target: "darwin-arm64", buildType: "metal" }), {
		backend: "localai-llamacpp",
		target: "darwin-arm64",
		shell: "bash",
		strategy: "darwin-llamacpp-grpc",
		binary: "llama-cpp-cpu-all",
		go_dynamic_loader: "none",
		grpc_dependency_mode: "default",
	});
	assert.deepEqual(buildPlan({ backend: "localai-whisper", target: "linux-amd64", buildType: "cpu" }), {
		backend: "localai-whisper",
		target: "linux-amd64",
		shell: "bash",
		strategy: "linux-go-build",
		binary: "whisper",
		go_dynamic_loader: "none",
		grpc_dependency_mode: "default",
	});
	assert.equal(
		buildPlan({ backend: "localai-whisper", target: "windows-amd64", buildType: "cpu" }).go_dynamic_loader,
		"xsys-windows",
	);
	assert.equal(
		buildPlan({ backend: "localai-whisper", target: "windows-amd64", buildType: "cpu" }).windows_library_name,
		"libgowhisper.dll",
	);
	assert.equal(
		buildPlan({ backend: "localai-vibevoice", target: "windows-amd64", buildType: "cpu" }).go_dynamic_loader,
		"xsys-windows",
	);
	assert.equal(
		buildPlan({ backend: "localai-vibevoice", target: "windows-amd64", buildType: "cpu" }).windows_library_name,
		"libgovibevoicecpp.dll",
	);
});

test("the join rejects every config-derived placeholder archive before adoption", async (t) => {
	const matrixRoot = await matrixArtifactFixture(t);
	for (const backend of config.backends) {
		for (const target of config.targets) {
			const archiveName = artifactArchiveName({ backend, target });
			for (const size of [minimumPublishedArchiveSizeBytes, minimumPublishedArchiveSizeBytes - 1]) {
				await writeFile(join(matrixRoot, archiveName), Buffer.alloc(size));
				assert.throws(
					() => createManifest({ config, artifactDirectory: matrixRoot, repository: "portpowered/infinite-you" }),
					new RegExp(`${backend.id}/${target.id} archive is placeholder-sized at ${size} bytes`),
				);
			}
			await writeFile(join(matrixRoot, archiveName), archiveFixtureBytes(backend, target));
		}
	}
});

test("placeholder rejection leaves the previously adopted bundle untouched", async (t) => {
	const matrixRoot = await matrixArtifactFixture(t);
	const placeholder = artifactArchiveName({ backend: config.backends[0], target: config.targets[0] });
	await writeFile(join(matrixRoot, placeholder), Buffer.alloc(minimumPublishedArchiveSizeBytes));

	const outputRoot = await mkdtemp(join(tmpdir(), "localai-backend-existing-release-"));
	t.after(() => rm(outputRoot, { recursive: true, force: true }));
	const previousManifest = join(outputRoot, "previous-manifest.json");
	await writeFile(previousManifest, "previous immutable release");

	assert.throws(
		() => writePublicationBundle({
			config,
			artifactDirectory: matrixRoot,
			outputDirectory: outputRoot,
			repository: "portpowered/infinite-you",
		}),
		/placeholder-sized/,
	);
	assert.deepEqual((await readdir(outputRoot)).sort(), ["previous-manifest.json"]);
	assert.equal(await readFile(previousManifest, "utf8"), "previous immutable release");
});

test("the workflow uses immutable actions, package inputs, and the pinned tag guard", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	for (const revision of Object.values(config.workflowPins)) assert.match(workflow, new RegExp(`@${revision}`));
	assert.doesNotMatch(workflow, /uses:\s+[^\n]+@(v\d|main|master|latest)\b/);
	assert.doesNotMatch(workflow, /update:\s*true/);
	assert.doesNotMatch(workflow, /apt-get/);
	assert.doesNotMatch(workflow, /brew install make\b/);
	assert.match(workflow, /windows_vcpkg_triplet/);
	assert.match(
		workflow,
		/vcpkg\.exe install grpc:\$\{\{ needs\.validate-inputs\.outputs\.windows_vcpkg_triplet \}\}[^\n]*--overlay-triplets=\$overlayTriplets/,
	);
	assert.match(workflow, /git\/ref\/tags/);
	assert.match(workflow, /git\/tags/);
	assert.match(workflow, /exists but its target could not be resolved/);
});

test("the Windows build plan resolves Git from the runner path bridge", async (t) => {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-windows-tools-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	const fakeCygpath = join(root, "cygpath");
	const fakeGitDirectory = join(root, "git-bin");
	const fakeGit = join(fakeGitDirectory, "git");
	await mkdir(fakeGitDirectory);
	await writeFile(
		fakeCygpath,
		"#!/usr/bin/env bash\n" +
			"path=\"$2\"\n" +
			"if command -v wslpath >/dev/null 2>&1; then\n" +
			"  wslpath -u \"$path\"\n" +
			"elif [[ \"$path\" =~ ^[A-Za-z]:\\\\ ]]; then\n" +
			"  drive=\"${path:0:1}\"\n" +
			"  rest=\"${path:2}\"\n" +
			"  printf '/%s/%s\\n' \"${drive,,}\" \"${rest//\\\\//}\"\n" +
			"else\n" +
			"  printf '%s\\n' \"$path\"\n" +
			"fi\n",
	);
	await writeFile(fakeGit, "#!/usr/bin/env bash\nexit 0\n");
	await chmod(fakeCygpath, 0o755);
	await chmod(fakeGit, 0o755);
	const shellRoot = (() => {
		if (process.platform !== "win32") return root;
		const converted = spawnSync("bash", ["-lc", `wslpath -u '${root.replaceAll("'", "'\\\"'\\\"'")}'`], { encoding: "utf8" });
		if (converted.status === 0 && converted.stdout.trim()) return converted.stdout.trim();
		return root.replace(/^([A-Za-z]):\\/, (_, drive) => `/${drive.toLowerCase()}/`).replaceAll("\\", "/");
	})();
	const shellEnvironment = {
		PATH: `${shellRoot}:${process.env.PATH ?? ""}`,
		WINDOWS_GIT_DIR: fakeGitDirectory,
	};
	const probe = spawnSync("bash", ["-c", "command -v cygpath || true"], {
		encoding: "utf8",
		env: { ...process.env, ...shellEnvironment },
	});
	if (!probe.stdout.trim().endsWith("/cygpath") && !probe.stdout.trim().endsWith("\\cygpath")) {
		t.skip("the available bash launcher does not expose the injected MSYS2 path bridge");
		return;
	}
	const plan = buildPlan({
		backend: "localai-whisper",
		target: "windows-amd64",
		buildType: "cpu",
		environment: shellEnvironment,
	});
	assert.match(plan.git.replaceAll("\\", "/"), /\/git-bin\/git$/);
});

function windowsBuildTimeout(workflow) {
	const jobTimeout = workflow.match(/^    timeout-minutes:\s+(\d+)\s*$/m);
	const windowsBuild = workflow.match(/^\s+- name: Build the backend package on Windows\s+([\s\S]*?)(?=^\s+- name: Build the backend package on Unix)/m);
	const unixBuild = workflow.match(/^\s+- name: Build the backend package on Unix\s+([\s\S]*?)(?=^\s+- name: Archive the Unix backend package)/m);
	if (!jobTimeout || !windowsBuild || !unixBuild) throw new Error("workflow is missing the build job or platform-specific backend build steps");
	const stepTimeout = windowsBuild[1].match(/^\s+timeout-minutes:\s+(\d+)\s*$/m);
	if (!stepTimeout) throw new Error("Windows backend build must have an explicit step timeout");
	const jobMinutes = Number(jobTimeout[1]);
	const windowsMinutes = Number(stepTimeout[1]);
	if (windowsMinutes < 45 || windowsMinutes > 300) throw new Error("Windows backend build timeout must be between 45 and 300 minutes");
	if (windowsMinutes >= jobMinutes) throw new Error("Windows backend build timeout must be below the build job timeout");
	if (/^\s+timeout-minutes:/m.test(unixBuild[1])) throw new Error("Darwin/Linux backend build must retain the job timeout");
	return { jobMinutes, windowsMinutes };
}

test("the Windows backend build has a bounded platform-specific step timeout", async () => {
	const workflow = await readFile(".github/workflows/localai-backend-artifacts.yml", "utf8");
	assert.deepEqual(windowsBuildTimeout(workflow), { jobMinutes: 360, windowsMinutes: 300 });
	assert.throws(
		() => windowsBuildTimeout(workflow.replace("timeout-minutes: 300", "timeout-minutes: 44")),
		/Windows backend build timeout must be between 45 and 300 minutes/,
	);
	assert.throws(
		() => windowsBuildTimeout(workflow.replace("timeout-minutes: 300", "timeout-minutes: 301")),
		/Windows backend build timeout must be between 45 and 300 minutes/,
	);
	assert.throws(
		() => windowsBuildTimeout(workflow.replace("timeout-minutes: 360", "timeout-minutes: 60")),
		/Windows backend build timeout must be below the build job timeout/,
	);
	assert.throws(
		() => windowsBuildTimeout(workflow.replace("        timeout-minutes: 300\n", "")),
		/Windows backend build must have an explicit step timeout/,
	);
	const buildScript = await readFile("scripts/build-localai-backend-artifact.sh", "utf8");
	assert.match(buildScript, /backend\/cpp\/grpc/);
	assert.match(buildScript, /compatibility_path="\$\{LOCALAI_ROOT\}\/backend\/grpc"/);
	assert.match(buildScript, /VCPKG_OVERLAY_TRIPLETS/);
	assert.match(buildScript, /windows_minimum_target="0x0A00"/);
	assert.match(buildScript, /-DCMAKE_CXX_FLAGS=-D_WIN32_WINNT=\$\{windows_minimum_target\}/);
	assert.match(buildScript, /grpc-server/);
	assert.match(buildScript, /llama-cpp-cpu-all/);
	assert.match(buildScript, /localai-backend-startup-smoke\.go/);
	const startupSmoke = await readFile("scripts/localai-backend-startup-smoke.go", "utf8");
	assert.match(startupSmoke, /LOCALAI_BACKEND_STARTUP_OK/);
	});

test("the Windows Go patch selects the staged DLL and adapts Whisper callbacks", async (t) => {
	const root = await mkdtemp(join(tmpdir(), "localai-backend-windows-patch-"));
	t.after(() => rm(root, { recursive: true, force: true }));
	const mainPath = join(root, "main.go");
	const loaderPath = join(root, "localai-backend-library_windows.go");
	await writeFile(
		mainPath,
		[
			"package main",
			"",
			"import (",
			'\t"os"',
			'\t"runtime"',
			'\t"github.com/ebitengine/purego"',
			")",
			"",
			"func onNewSegment(int32, int32, uintptr) {}",
			"",
			"func main() {",
			'\tlibName := os.Getenv("WHISPER_LIBRARY")',
			'\tif libName == "" {',
			'\t\tif runtime.GOOS == "darwin" {',
			'\t\t\tlibName = "./libgowhisper-fallback.dylib"',
			'\t\t} else {',
			'\t\t\tlibName = "./libgowhisper-fallback.so"',
			"\t\t}",
			"\t}",
			"\tgosd, err := purego.Dlopen(libName, purego.RTLD_NOW|purego.RTLD_GLOBAL)",
			"\t_ = gosd",
			"\t_ = err",
			"\t_ = purego.NewCallback(onNewSegment)",
			"}",
			"",
		].join("\n"),
	);

	patchWindowsGoLoader({ mainPath, loaderPath, libraryName: "libgowhisper.dll", backendID: "localai-whisper" });
	const patchedMain = await readFile(mainPath, "utf8");
	const loader = await readFile(loaderPath, "utf8");
	assert.match(patchedMain, /loadBackendLibrary\(libName\)/);
	assert.match(patchedMain, /runtime\.GOOS == "windows"/);
	assert.match(patchedMain, /libName = "\.\/libgowhisper\.dll"/);
	assert.match(patchedMain, /purego\.NewCallback\(localAINewSegmentCallback\)/);
	assert.match(loader, /\/\/go:build windows/);
	assert.match(loader, /windows\.LoadLibrary/);
	assert.match(loader, /func localAINewSegmentCallback\([^)]*\) uintptr/);
});

test("manifest verification rejects bytes tampered after manifest creation", async (t) => {
	const root = await matrixArtifactFixture(t);
	const manifest = createManifest({ config, artifactDirectory: root, repository: "portpowered/infinite-you" });
	const archiveName = manifest.artifacts[0].artifact.name;
	await writeFile(join(root, archiveName), Buffer.alloc(minimumPublishedArchiveSizeBytes + 1, 0x43));
	assert.throws(
		() => verifyManifestArchives({ config, manifest, artifactDirectory: root }),
		/integrity mismatch/,
	);
});
