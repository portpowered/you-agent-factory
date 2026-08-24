import { appendFileSync, copyFileSync, mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join, normalize, relative, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const defaultConfigPath = join(repositoryRoot, ".github", "localai-backend-artifacts.json");
const shaPattern = /^[0-9a-f]{40}$/;
const digestPattern = /^[0-9a-f]{64}$/;
const versionPattern = /^\d+\.\d+(?:\.\d+)?$/;
const repositoryPattern = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;
const supportedAccelerators = new Set(["cpu", "metal"]);

// A published backend archive must contain a credible runnable payload. The
// strict boundary is intentionally shared by manifest creation and final
// publication verification so a placeholder cannot be adopted at either
// artifact-side boundary.
export const minimumPublishedArchiveSizeBytes = 1 << 20;

const expectedBackendIds = ["localai-llamacpp", "localai-whisper", "localai-vibevoice"];
const expectedTargetIds = ["darwin-arm64", "linux-amd64", "windows-amd64"];
const expectedWorkflowPins = {
	checkout: "11bd71901bbe5b1630ceea73d27597364c9af683",
	setupNode: "49933ea5288caeca8642d1e84afbd3f7d6820020",
	setupGo: "d35c59abb061a4a6fb18e82ac0862c26744d6ab5",
	getCMake: "fffaaafeea488556c2c12dad60690008bc1caacb",
	setupMsys2: "fb197b72ce45fb24f17bf3f807a388985654d1f2",
	uploadArtifact: "ea165f8d65b6e75b540449e92b4886f43607fa02",
	downloadArtifact: "d3f86a106a0bac45b974a628896c90dbdf5c8093",
};
const expectedWindowsMsysPackages = [
	"make=4.4.1-3",
	"mingw-w64-x86_64-make=4.4.1-5",
	"mingw-w64-x86_64-binutils=2.47-3",
	"mingw-w64-x86_64-gcc=16.2.0-3",
	"mingw-w64-x86_64-ninja=1.13.2-1",
];

const expectedBackendFacts = {
	"localai-llamacpp": {
		sourcePath: "backend/cpp/llama-cpp",
		sourceRepository: "https://github.com/ggerganov/llama.cpp",
		sourcePinVariable: "LLAMA_VERSION",
		makeTarget: "package",
		binary: "llama-cpp-cpu-all",
	},
	"localai-whisper": {
		sourcePath: "backend/go/whisper",
		sourceRepository: "https://github.com/ggml-org/whisper.cpp",
		sourcePinVariable: "WHISPER_CPP_VERSION",
		makeTarget: "package",
		binary: "whisper",
	},
	"localai-vibevoice": {
		sourcePath: "backend/go/vibevoice-cpp",
		sourceRepository: "https://github.com/mudler/vibevoice.cpp",
		sourcePinVariable: "VIBEVOICE_CPP_VERSION",
		makeTarget: "package",
		binary: "vibevoice-cpp",
	},
};

const targetHostFacts = {
	"darwin-arm64": { platform: "darwin", architectures: ["arm64"] },
	"linux-amd64": { platform: "linux", architectures: ["x64", "amd64"] },
	"windows-amd64": { platform: "win32", architectures: ["x64", "amd64"] },
};

const expectedTargetFacts = {
	"darwin-arm64": { os: "darwin", architecture: "arm64", runner: "macos-14", buildType: "metal", accelerators: ["metal"] },
	"linux-amd64": { os: "linux", architecture: "amd64", runner: "ubuntu-24.04", buildType: "cpu", accelerators: ["cpu"] },
	"windows-amd64": { os: "windows", architecture: "amd64", runner: "windows-2022", buildType: "cpu", accelerators: ["cpu"] },
};

export function loadConfig(configPath = defaultConfigPath) {
	return JSON.parse(readFileSync(configPath, "utf8"));
}

function isPlainObject(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function addError(errors, message) {
	errors.push(message);
}

function validateSha(errors, value, label) {
	if (typeof value !== "string" || !shaPattern.test(value)) {
		addError(errors, `${label} must be a lowercase 40-character commit or blob SHA`);
	}
}

function validateVersion(errors, value, label) {
	if (typeof value !== "string" || !versionPattern.test(value)) {
		addError(errors, `${label} must be an exact numeric tool version`);
	}
}

function validatePackageSpec(errors, value, label) {
	if (typeof value !== "string" || !/^[A-Za-z0-9_.+-]+=[0-9][A-Za-z0-9.+:~_-]*$/.test(value)) {
		addError(errors, `${label} must be an exact package name and version`);
	}
}

function validateAccelerators(errors, value, label) {
	if (!Array.isArray(value) || value.length === 0) {
		addError(errors, `${label} must contain at least one accelerator`);
		return;
	}
	const seen = new Set();
	for (const accelerator of value) {
		if (typeof accelerator !== "string" || !supportedAccelerators.has(accelerator)) {
			addError(errors, `${label} contains unsupported accelerator ${accelerator}`);
		}
		if (seen.has(accelerator)) addError(errors, `${label} contains duplicate ${accelerator}`);
		seen.add(accelerator);
	}
}

function validateRelativePath(errors, value, label) {
	if (typeof value !== "string" || value.length === 0 || value.includes("\\")) {
		addError(errors, `${label} must be a non-empty POSIX relative path`);
		return;
	}
	const normalized = normalize(value).replaceAll("\\", "/");
	if (normalized !== value || value.startsWith("/") || value.split("/").includes("..")) {
		addError(errors, `${label} must not escape the checkout`);
	}
}

function validateUniqueIds(errors, values, label) {
	const seen = new Set();
	for (const value of values) {
		if (seen.has(value)) addError(errors, `${label} contains duplicate ${value}`);
		seen.add(value);
	}
}

function validateBackend(errors, backend) {
	if (!isPlainObject(backend)) {
		addError(errors, "each backend entry must be an object");
		return;
	}
	for (const field of ["id", "sourcePath", "sourceRepository", "sourceCommit", "sourcePinVariable", "makeTarget", "binary"]) {
		if (typeof backend[field] !== "string" || backend[field].length === 0) {
			addError(errors, `backend ${field} must be non-empty`);
		}
	}
	validateRelativePath(errors, backend.sourcePath, `backend ${backend.id ?? "<unknown>"} sourcePath`);
	validateSha(errors, backend.sourceCommit, `backend ${backend.id ?? "<unknown>"} sourceCommit`);
	if (typeof backend.sourceRepository === "string" && !/^https:\/\/github\.com\/[^/]+\/[^/]+(?:\.git)?$/.test(backend.sourceRepository)) {
		addError(errors, `backend ${backend.id ?? "<unknown>"} sourceRepository must be an immutable GitHub HTTPS repository URL`);
	}
	if (backend.makeTarget !== "package") {
		addError(errors, `backend ${backend.id ?? "<unknown>"} must use the package build target`);
	}
	if (typeof backend.binary === "string" && /[\\/]/.test(backend.binary)) {
		addError(errors, `backend ${backend.id ?? "<unknown>"} binary must be a file name`);
	}
	const expected = expectedBackendFacts[backend.id];
	if (expected) {
		for (const [field, value] of Object.entries(expected)) {
			if (backend[field] !== value) addError(errors, `backend ${backend.id} ${field} must be ${value}`);
		}
	}
}

function validateTarget(errors, target) {
	if (!isPlainObject(target)) {
		addError(errors, "each target entry must be an object");
		return;
	}
	for (const field of ["id", "os", "architecture", "runner", "buildType"]) {
		if (typeof target[field] !== "string" || target[field].length === 0) {
			addError(errors, `target ${field} must be non-empty`);
		}
	}
	validateAccelerators(errors, target.accelerators, `target ${target.id ?? "<unknown>"} accelerators`);
	if (typeof target.runner === "string" && /latest|master|main/i.test(target.runner)) {
		addError(errors, `target ${target.id ?? "<unknown>"} runner must not be floating`);
	}
	if (target.os !== "darwin" && target.os !== "linux" && target.os !== "windows") {
		addError(errors, `target ${target.id ?? "<unknown>"} has unsupported operating system ${target.os}`);
	}
	if (target.architecture !== "arm64" && target.architecture !== "amd64") {
		addError(errors, `target ${target.id ?? "<unknown>"} has unsupported architecture ${target.architecture}`);
	}
	const expected = expectedTargetFacts[target.id];
	if (expected && (target.os !== expected.os || target.architecture !== expected.architecture || target.runner !== expected.runner || target.buildType !== expected.buildType || JSON.stringify(target.accelerators) !== JSON.stringify(expected.accelerators))) {
		addError(errors, `target ${target.id} does not match its pinned OS, architecture, and build type`);
	}
}

function validateWorkflowPins(errors, pins) {
	if (!isPlainObject(pins)) {
		addError(errors, "workflowPins must be an object");
		return;
	}
	for (const [name, expected] of Object.entries(expectedWorkflowPins)) {
		validateSha(errors, pins[name], `workflowPins.${name}`);
		if (pins[name] !== expected) addError(errors, `workflowPins.${name} must be the repository-approved immutable action revision`);
	}
}

function validateHostToolchain(errors, hostToolchain) {
	if (!isPlainObject(hostToolchain)) {
		addError(errors, "hostToolchain must be an object");
		return;
	}
	const linux = hostToolchain.linux;
	if (!isPlainObject(linux)) {
		addError(errors, "hostToolchain.linux must be an object");
	} else {
		for (const key of ["gccVersion", "makeVersion", "ninjaVersion", "pkgConfigVersion"]) {
			validateVersion(errors, linux[key], `hostToolchain.linux.${key}`);
		}
	}
	const macos = hostToolchain.macos;
	if (!isPlainObject(macos)) {
		addError(errors, "hostToolchain.macos must be an object");
	} else {
		validateSha(errors, macos.makeFormulaCommit, "hostToolchain.macos.makeFormulaCommit");
		validateVersion(errors, macos.makeVersion, "hostToolchain.macos.makeVersion");
	}
	const windows = hostToolchain.windows;
	if (!isPlainObject(windows)) {
		addError(errors, "hostToolchain.windows must be an object");
	} else {
		if (windows.vcpkgTriplet !== "x64-mingw-static-release") addError(errors, "hostToolchain.windows.vcpkgTriplet must be x64-mingw-static-release");
		if (JSON.stringify(windows.msysPackages) !== JSON.stringify(expectedWindowsMsysPackages)) {
			addError(errors, "hostToolchain.windows.msysPackages must be the pinned native build package set");
		}
		if (Array.isArray(windows.msysPackages)) {
			windows.msysPackages.forEach((value, index) => validatePackageSpec(errors, value, `hostToolchain.windows.msysPackages[${index}]`));
		}
	}
}

export function matrixForConfig(config) {
	return {
		include: config.backends.flatMap((backend) =>
			config.targets.map((target) => ({
				backend: backend?.id,
				target: target?.id,
				runner: target?.runner,
				os: target?.os,
				architecture: target?.architecture,
				build_type: target?.buildType,
				accelerators: target?.accelerators,
				source_path: backend?.sourcePath,
				source_repository: backend?.sourceRepository,
				source_commit: backend?.sourceCommit,
				source_pin_variable: backend?.sourcePinVariable,
				make_target: backend?.makeTarget,
				binary: backend?.binary,
			})),
		),
	};
}

export function validateConfig(config) {
	const errors = [];
	if (!isPlainObject(config)) return { errors: ["configuration must be an object"], matrix: null };
	if (config.schemaVersion !== 1) addError(errors, "schemaVersion must be 1");
	validateVersion(errors, config.nodeVersion, "nodeVersion");
	if (config.localaiRepository !== "https://github.com/mudler/LocalAI.git") {
		addError(errors, "localaiRepository must be the repository-owned LocalAI upstream");
	}
	validateSha(errors, config.localaiCommit, "localaiCommit");
	validateRelativePath(errors, config.protocolPath, "protocolPath");
	if (config.protocolPath !== "backend/backend.proto") addError(errors, "protocolPath must be backend/backend.proto");
	validateSha(errors, config.protocolRevision, "protocolRevision");
	validateSha(errors, config.grpcCommit, "grpcCommit");
	validateSha(errors, config.vcpkgCommit, "vcpkgCommit");
	validateWorkflowPins(errors, config.workflowPins);
	validateHostToolchain(errors, config.hostToolchain);

	if (!isPlainObject(config.toolchain)) {
		addError(errors, "toolchain must be an object");
	} else {
		for (const [key, value] of Object.entries(config.toolchain)) validateVersion(errors, value, `toolchain.${key}`);
		for (const key of ["goVersion", "cmakeVersion", "protobufVersion", "grpcVersion"]) {
			if (!(key in config.toolchain)) addError(errors, `toolchain.${key} is required`);
		}
	}

	if (!Array.isArray(config.backends)) {
		addError(errors, "backends must be an array");
	} else {
		validateUniqueIds(errors, config.backends.map((backend) => backend?.id), "backends");
		if (JSON.stringify(config.backends.map((backend) => backend?.id)) !== JSON.stringify(expectedBackendIds)) {
			addError(errors, `backends must be exactly ${expectedBackendIds.join(", ")}`);
		}
		for (const backend of config.backends) validateBackend(errors, backend);
	}

	if (!Array.isArray(config.targets)) {
		addError(errors, "targets must be an array");
	} else {
		validateUniqueIds(errors, config.targets.map((target) => target?.id), "targets");
		if (JSON.stringify(config.targets.map((target) => target?.id)) !== JSON.stringify(expectedTargetIds)) {
			addError(errors, `targets must be exactly ${expectedTargetIds.join(", ")}`);
		}
		for (const target of config.targets) validateTarget(errors, target);
	}

	const matrix = Array.isArray(config.backends) && Array.isArray(config.targets) ? matrixForConfig(config) : null;
	if (matrix) {
		const combinations = matrix.include.map((entry) => `${entry.backend}/${entry.target}`);
		validateUniqueIds(errors, combinations, "matrix");
		if (matrix.include.length !== expectedBackendIds.length * expectedTargetIds.length) {
			addError(errors, "matrix must contain exactly nine backend/target combinations");
		}
	}
	return { errors, matrix };
}

function runGit(root, args) {
	return execFileSync("git", ["-C", root, ...args], { encoding: "utf8" }).trim();
}

function readPinnedMakeVariable(makefile, variable) {
	const match = makefile.match(new RegExp(`^${variable}\\?=\\s*([^\\s#]+)`, "m"));
	return match?.[1] ?? "";
}

function assertPathInside(root, candidate, label) {
	const resolvedRoot = resolve(root);
	const resolvedCandidate = resolve(candidate);
	const relativePath = relative(resolvedRoot, resolvedCandidate);
	if (relativePath.startsWith("..") || relativePath.includes("/../") || relativePath.includes("\\..")) {
		throw new Error(`${label} escapes the LocalAI checkout`);
	}
}

export function verifySource({
	config,
	localaiRoot,
	backendId,
	targetId,
	}) {
	const result = validateConfig(config);
	if (result.errors.length > 0) throw new Error(result.errors.join("; "));
	const backend = config.backends.find((entry) => entry.id === backendId);
	const target = config.targets.find((entry) => entry.id === targetId);
	if (!backend || !target) throw new Error(`unknown backend/target ${backendId}/${targetId}`);
	if (!statSync(localaiRoot).isDirectory()) throw new Error(`LocalAI checkout is missing: ${localaiRoot}`);
	if (runGit(localaiRoot, ["rev-parse", "HEAD"]) !== config.localaiCommit) {
		throw new Error(`LocalAI checkout is not at pinned commit ${config.localaiCommit}`);
	}
	const protocolPath = join(localaiRoot, config.protocolPath);
	assertPathInside(localaiRoot, protocolPath, "protocol path");
	if (runGit(localaiRoot, ["hash-object", config.protocolPath]) !== config.protocolRevision) {
		throw new Error(`protocol blob does not match pinned revision ${config.protocolRevision}`);
	}
	const makefilePath = join(localaiRoot, backend.sourcePath, "Makefile");
	assertPathInside(localaiRoot, makefilePath, "backend Makefile");
	const makefile = readFileSync(makefilePath, "utf8");
	const sourceCommit = readPinnedMakeVariable(makefile, backend.sourcePinVariable);
	if (sourceCommit !== backend.sourceCommit) {
		throw new Error(`${backendId} Makefile ${backend.sourcePinVariable} is ${sourceCommit || "missing"}, expected ${backend.sourceCommit}`);
	}
	const sourceRepositoryVariable = backend.sourceRepository.includes("llama.cpp")
		? "LLAMA_REPO"
		: backend.sourceRepository.includes("whisper.cpp")
			? "WHISPER_REPO"
			: "VIBEVOICE_REPO";
	const sourceRepository = readPinnedMakeVariable(makefile, sourceRepositoryVariable);
	if (sourceRepository !== backend.sourceRepository) {
		throw new Error(`${backendId} Makefile ${sourceRepositoryVariable} is ${sourceRepository || "missing"}, expected ${backend.sourceRepository}`);
	}
	const host = targetHostFacts[targetId];
	if (host && (process.platform !== host.platform || !host.architectures.includes(process.arch))) {
		throw new Error(`runner host ${process.platform}/${process.arch} does not match ${targetId}`);
	}
	return { backend, target, protocolRevision: config.protocolRevision };
}

export function buildMetadata({ config, localaiRoot, backendId, targetId }) {
	const verified = verifySource({ config, localaiRoot, backendId, targetId });
	return {
		formatVersion: 1,
		backend: verified.backend.id,
		target: {
			id: verified.target.id,
			operatingSystem: verified.target.os,
			architecture: verified.target.architecture,
			buildType: verified.target.buildType,
			accelerators: [...verified.target.accelerators],
		},
		source: {
			repository: config.localaiRepository,
			commit: config.localaiCommit,
			path: verified.backend.sourcePath,
			backendRepository: verified.backend.sourceRepository,
			backendCommit: verified.backend.sourceCommit,
			backendPinVariable: verified.backend.sourcePinVariable,
		},
		protocol: {
			path: config.protocolPath,
			revision: config.protocolRevision,
		},
		toolchain: {
			...config.toolchain,
			grpcCommit: config.grpcCommit,
			vcpkgCommit: config.vcpkgCommit,
		},
		buildInputs: {
			nodeVersion: config.nodeVersion,
			actionPins: { ...config.workflowPins },
			hostToolchain: structuredClone(config.hostToolchain),
		},
		payload: {
			binary: verified.backend.binary,
			makeTarget: verified.backend.makeTarget,
		},
	};
}

export function verifyPayload({ packageRoot, binary, targetId }) {
	if (!expectedTargetFacts[targetId]) throw new Error(`unsupported payload target ${targetId}`);
	if (!statSync(packageRoot).isDirectory()) throw new Error(`package directory is missing: ${packageRoot}`);
	const files = listFilesWithDirectory(packageRoot);
	if (files.length === 0 || files.every((entry) => entry.size === 0)) {
		throw new Error("backend package has no non-empty payload");
	}
	const binaryPath = [join(packageRoot, binary), join(packageRoot, `${binary}.exe`)].find((path) => {
		try {
			return statSync(path).isFile() && statSync(path).size > 0;
		} catch {
			return false;
		}
	});
	if (!binaryPath) throw new Error(`backend package is missing non-empty executable ${binary}`);
	const bytes = readFileSync(binaryPath);
	if (targetId === "windows-amd64") {
		if (bytes.length < 64 || bytes[0] !== 0x4d || bytes[1] !== 0x5a) throw new Error(`${binary} is not a Windows PE executable`);
		const peOffset = bytes.readUInt32LE(0x3c);
		if (bytes.length < peOffset + 6 || bytes.toString("ascii", peOffset, peOffset + 4) !== "PE\u0000\u0000" || bytes.readUInt16LE(peOffset + 4) !== 0x8664) {
			throw new Error(`${binary} is not an amd64 Windows PE executable`);
		}
	} else if (targetId === "linux-amd64") {
		if (bytes.length < 20 || bytes[0] !== 0x7f || bytes.toString("ascii", 1, 4) !== "ELF" || bytes[4] !== 2 || bytes[5] !== 1 || bytes[18] !== 0x3e) {
			throw new Error(`${binary} is not an amd64 Linux ELF executable`);
		}
	} else if (targetId === "darwin-arm64") {
		if (!isArm64MachO(bytes)) throw new Error(`${binary} is not an arm64 Darwin Mach-O executable`);
	}
	return { binaryPath, files, bytes: bytes.length };
}

function isArm64MachO(bytes) {
	if (bytes.length < 8) return false;
	const littleMagic = bytes.readUInt32LE(0);
	const littleCPUType = bytes.readUInt32LE(4);
	if (littleMagic === 0xfeedfacf) return littleCPUType === 0x0100000c;

	const bigMagic = bytes.readUInt32BE(0);
	const isBigEndianFat = bigMagic === 0xcafebabe || bigMagic === 0xcafebabf;
	const isLittleEndianFat = littleMagic === 0xcafebabe || littleMagic === 0xcafebabf;
	if (!isBigEndianFat && !isLittleEndianFat) return false;

	const read = isBigEndianFat ? (offset) => bytes.readUInt32BE(offset) : (offset) => bytes.readUInt32LE(offset);
	const isFat64 = isBigEndianFat ? bigMagic === 0xcafebabf : littleMagic === 0xcafebabf;
	const architectureCount = read(4);
	const architectureSize = isFat64 ? 32 : 20;
	if (architectureCount === 0 || architectureCount > 128 || 8 + architectureCount * architectureSize > bytes.length) return false;
	for (let index = 0; index < architectureCount; index += 1) {
		const offset = 8 + index * architectureSize;
		const sliceOffset = isFat64 ? Number((isBigEndianFat ? bytes.readBigUInt64BE(offset + 8) : bytes.readBigUInt64LE(offset + 8))) : read(offset + 8);
		const sliceSize = isFat64 ? Number((isBigEndianFat ? bytes.readBigUInt64BE(offset + 16) : bytes.readBigUInt64LE(offset + 16))) : read(offset + 12);
		if (!Number.isSafeInteger(sliceOffset) || !Number.isSafeInteger(sliceSize) || sliceSize === 0 || sliceOffset + sliceSize > bytes.length) continue;
		if (read(offset) === 0x0100000c) return true;
	}
	return false;
}

function listFilesWithDirectory(root) {
	const files = [];
	const visit = (directory) => {
		for (const entry of readdirSync(directory)) {
			const path = join(directory, entry);
			const info = statSync(path);
			if (info.isDirectory()) visit(path);
			else files.push({ path, size: info.size });
		}
	};
	visit(root);
	return files;
}

function validateRepository(repository) {
	if (typeof repository !== "string" || !repositoryPattern.test(repository)) {
		throw new Error(`repository must be an owner/name identifier, received ${repository || "<missing>"}`);
	}
}

function canonicalPinDocument(config) {
	return {
		localaiRepository: config.localaiRepository,
		localaiCommit: config.localaiCommit,
		protocolPath: config.protocolPath,
		protocolRevision: config.protocolRevision,
		grpcCommit: config.grpcCommit,
		vcpkgCommit: config.vcpkgCommit,
		toolchain: Object.fromEntries(Object.entries(config.toolchain).sort(([left], [right]) => left.localeCompare(right))),
		workflowPins: Object.fromEntries(Object.entries(config.workflowPins).sort(([left], [right]) => left.localeCompare(right))),
		hostToolchain: config.hostToolchain,
		nodeVersion: config.nodeVersion,
		backends: config.backends.map(({ id, sourceRepository, sourceCommit, sourcePinVariable }) => ({ id, sourceRepository, sourceCommit, sourcePinVariable })),
		targets: config.targets.map(({ id, os, architecture, buildType, accelerators }) => ({ id, os, architecture, buildType, accelerators })),
	};
}

export function publicationIdentity(config) {
	const result = validateConfig(config);
	if (result.errors.length > 0) throw new Error(result.errors.join("; "));
	const pinFingerprint = createHash("sha256")
		.update(JSON.stringify(canonicalPinDocument(config)))
		.digest("hex");
	return {
		pinFingerprint,
		releaseTag: `localai-backends-v${config.schemaVersion}-${pinFingerprint}`,
	};
}

export function artifactArchiveName({ backend, target }) {
	const extension = target.id === "windows-amd64" ? "zip" : "tar.gz";
	return `localai-backend-${backend.id}-${target.id}-${backend.sourceCommit}.${extension}`;
}

function artifactMetadataName(archiveName) {
	return `${archiveName}.metadata.json`;
}

function expectedArtifactEntries(config) {
	return config.backends.flatMap((backend) =>
		config.targets.map((target) => ({
			backend,
			target,
			key: `${backend.id}/${target.id}`,
			archiveName: artifactArchiveName({ backend, target }),
		})),
	);
}

function listArtifactDirectoryFiles(directory) {
	if (!statSync(directory).isDirectory()) throw new Error(`artifact directory is missing: ${directory}`);
	return readdirSync(directory).sort().map((name) => {
		const path = join(directory, name);
		if (!statSync(path).isFile()) throw new Error(`artifact directory contains a non-file entry: ${name}`);
		return name;
	});
}

function readJsonFile(path, label) {
	try {
		return JSON.parse(readFileSync(path, "utf8"));
	} catch (error) {
		throw new Error(`${label} is not valid JSON: ${error.message}`);
	}
}

function metadataField(metadata, path) {
	return path.split(".").reduce((value, key) => value?.[key], metadata);
}

function assertMetadataField(metadata, path, expected, label) {
	const actual = metadataField(metadata, path);
	if (JSON.stringify(actual) !== JSON.stringify(expected)) {
		throw new Error(`${label} ${path} is ${JSON.stringify(actual)}, expected ${JSON.stringify(expected)}`);
	}
}

function validateMatrixMetadata(metadata, { config, backend, target, key }) {
	if (!isPlainObject(metadata)) throw new Error(`${key} metadata must be an object`);
	const expectedFields = {
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
	};
	for (const [path, expected] of Object.entries(expectedFields)) assertMetadataField(metadata, path, expected, `${key} metadata mismatch:`);
	for (const [name, expected] of Object.entries(config.toolchain)) assertMetadataField(metadata, `toolchain.${name}`, expected, `${key} metadata mismatch:`);
	assertMetadataField(metadata, "toolchain.grpcCommit", config.grpcCommit, `${key} metadata mismatch:`);
	assertMetadataField(metadata, "toolchain.vcpkgCommit", config.vcpkgCommit, `${key} metadata mismatch:`);
	assertMetadataField(metadata, "buildInputs.nodeVersion", config.nodeVersion, `${key} metadata mismatch:`);
	assertMetadataField(metadata, "buildInputs.actionPins", config.workflowPins, `${key} metadata mismatch:`);
	assertMetadataField(metadata, "buildInputs.hostToolchain", config.hostToolchain, `${key} metadata mismatch:`);
}

function readArchiveDigest(path, label) {
	const bytes = readFileSync(path);
	if (bytes.length <= minimumPublishedArchiveSizeBytes) {
		throw new Error(
			`${label} is placeholder-sized at ${bytes.length} bytes; must be strictly greater than ${minimumPublishedArchiveSizeBytes} bytes (1 MiB)`,
		);
	}
	return {
		sizeBytes: bytes.length,
		sha256: createHash("sha256").update(bytes).digest("hex"),
	};
}

export function verifyManifestArchives({ config, manifest, artifactDirectory }) {
	const expected = expectedArtifactEntries(config);
	if (!isPlainObject(manifest) || !Array.isArray(manifest.artifacts) || manifest.artifacts.length !== expected.length) {
		throw new Error(`manifest must contain exactly ${expected.length} backend artifacts`);
	}
	const expectedByID = new Map(expected.map((entry) => [entry.key, entry]));
	const seenIds = new Set();
	const seenNames = new Set();
	for (const entry of manifest.artifacts) {
		const id = entry?.id;
		const name = entry?.artifact?.name;
		if (typeof id !== "string" || seenIds.has(id)) throw new Error(`manifest contains duplicate artifact identity ${id || "<missing>"}`);
		const expectedEntry = expectedByID.get(id);
		if (!expectedEntry) throw new Error(`manifest contains unexpected artifact identity ${id || "<missing>"}`);
		if (name !== expectedEntry.archiveName) {
			throw new Error(`${id} archive name ${name || "<missing>"} does not match the publication configuration ${expectedEntry.archiveName}`);
		}
		if (typeof name !== "string" || !/^[A-Za-z0-9][A-Za-z0-9._-]+$/.test(name) || seenNames.has(name)) {
			throw new Error(`manifest contains duplicate or unsafe archive identity ${name || "<missing>"}`);
		}
		seenIds.add(id);
		seenNames.add(name);
		if (!Number.isSafeInteger(entry.artifact.sizeBytes) || entry.artifact.sizeBytes <= 0) {
			throw new Error(`${id} manifest sizeBytes must be a positive integer`);
		}
		if (typeof entry.artifact.sha256 !== "string" || !digestPattern.test(entry.artifact.sha256)) {
			throw new Error(`${id} manifest sha256 must be a lowercase 64-character digest`);
		}
		const archivePath = join(artifactDirectory, name);
		assertPathInside(artifactDirectory, archivePath, `${id} archive`);
		const observed = readArchiveDigest(archivePath, `${id} archive`);
		if (observed.sizeBytes !== entry.artifact.sizeBytes || observed.sha256 !== entry.artifact.sha256) {
			throw new Error(`${id} archive integrity mismatch: manifest size/digest do not match published bytes`);
		}
	}
	return true;
}

export function createManifest({ config, artifactDirectory, repository }) {
	validateRepository(repository);
	const result = validateConfig(config);
	if (result.errors.length > 0) throw new Error(result.errors.join("; "));
	const identity = publicationIdentity(config);
	const expected = expectedArtifactEntries(config);
	const expectedFiles = expected.flatMap(({ archiveName }) => [archiveName, artifactMetadataName(archiveName)]).sort();
	const actualFiles = listArtifactDirectoryFiles(artifactDirectory);
	if (JSON.stringify(actualFiles) !== JSON.stringify(expectedFiles)) {
		throw new Error(`matrix artifact set must contain exactly the nine archives and nine provenance sidecars; expected ${expectedFiles.join(", ")}, received ${actualFiles.join(", ")}`);
	}
	const artifacts = expected.map(({ backend, target, key, archiveName }) => {
		const metadata = readJsonFile(join(artifactDirectory, artifactMetadataName(archiveName)), `${key} metadata`);
		validateMatrixMetadata(metadata, { config, backend, target, key });
		const digest = readArchiveDigest(join(artifactDirectory, archiveName), `${key} archive`);
		return {
			id: key,
			backend: {
				id: backend.id,
				source: {
					repository: backend.sourceRepository,
					commit: backend.sourceCommit,
					pinVariable: backend.sourcePinVariable,
				},
			},
			source: {
				repository: config.localaiRepository,
				commit: config.localaiCommit,
				path: backend.sourcePath,
			},
			protocol: {
				path: config.protocolPath,
				revision: config.protocolRevision,
			},
			target: {
				id: target.id,
				operatingSystem: target.os,
				architecture: target.architecture,
				accelerators: [...target.accelerators],
			},
			artifact: {
				name: archiveName,
				location: `https://github.com/${repository}/releases/download/${identity.releaseTag}/${archiveName}`,
				sizeBytes: digest.sizeBytes,
				sha256: digest.sha256,
			},
		};
	});
	const manifest = {
		schemaVersion: 1,
		kind: "localai-backend-artifacts",
		publication: {
			id: identity.releaseTag,
			releaseTag: identity.releaseTag,
			repository,
			pinFingerprint: identity.pinFingerprint,
			source: {
				repository: config.localaiRepository,
				commit: config.localaiCommit,
			},
			protocol: {
				path: config.protocolPath,
				revision: config.protocolRevision,
			},
			toolchain: {
				...config.toolchain,
				grpcCommit: config.grpcCommit,
				vcpkgCommit: config.vcpkgCommit,
			},
		},
		artifacts,
	};
	verifyManifestArchives({ config, manifest, artifactDirectory });
	return manifest;
}

export function writePublicationBundle({ config, artifactDirectory, outputDirectory, repository }) {
	const manifest = createManifest({ config, artifactDirectory, repository });
	mkdirSync(outputDirectory, { recursive: true });
	if (readdirSync(outputDirectory).length > 0) throw new Error(`publication directory must be empty: ${outputDirectory}`);
	for (const entry of manifest.artifacts) copyFileSync(join(artifactDirectory, entry.artifact.name), join(outputDirectory, entry.artifact.name));
	writeFileSync(join(outputDirectory, "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);
	verifyManifestArchives({ config, manifest, artifactDirectory: outputDirectory });
	return { manifest, outputDirectory };
}

function parseOption(args, name) {
	const index = args.indexOf(name);
	if (index < 0 || !args[index + 1]) throw new Error(`${name} is required`);
	return args[index + 1];
}

function main(argv) {
	const command = argv[0];
	if (command === "matrix" || command === "validate") {
		const configPath = argv.includes("--config") ? parseOption(argv, "--config") : defaultConfigPath;
		const config = loadConfig(configPath);
		const result = validateConfig(config);
		if (result.errors.length > 0) throw new Error(result.errors.join("\n"));
		if (command === "matrix") {
			process.stdout.write(`${JSON.stringify(result.matrix)}\n`);
			return;
		}
		const outputPath = argv.includes("--github-output") ? parseOption(argv, "--github-output") : "";
		if (outputPath) {
			appendFileSync(outputPath, `matrix=${JSON.stringify(result.matrix)}\n`);
			appendFileSync(outputPath, `node_version=${config.nodeVersion}\n`);
			appendFileSync(outputPath, `localai_commit=${config.localaiCommit}\n`);
			appendFileSync(outputPath, `protocol_revision=${config.protocolRevision}\n`);
			appendFileSync(outputPath, `grpc_commit=${config.grpcCommit}\n`);
			appendFileSync(outputPath, `vcpkg_commit=${config.vcpkgCommit}\n`);
			appendFileSync(outputPath, `go_version=${config.toolchain.goVersion}\n`);
			appendFileSync(outputPath, `cmake_version=${config.toolchain.cmakeVersion}\n`);
			appendFileSync(outputPath, `protobuf_version=${config.toolchain.protobufVersion}\n`);
			appendFileSync(outputPath, `grpc_version=${config.toolchain.grpcVersion}\n`);
			appendFileSync(outputPath, `macos_make_formula_commit=${config.hostToolchain.macos.makeFormulaCommit}\n`);
			appendFileSync(outputPath, `macos_make_version=${config.hostToolchain.macos.makeVersion}\n`);
			appendFileSync(outputPath, `windows_msys_packages=${config.hostToolchain.windows.msysPackages.join(" ")}\n`);
			appendFileSync(outputPath, `windows_vcpkg_triplet=${config.hostToolchain.windows.vcpkgTriplet}\n`);
		}
		process.stdout.write(`LOCALAI_BACKEND_ARTIFACT_INPUTS_OK combinations=${result.matrix.include.length}\n`);
		return;
	}
	if (command === "verify-source") {
		const config = loadConfig(argv.includes("--config") ? parseOption(argv, "--config") : defaultConfigPath);
		const verified = verifySource({
			config,
			localaiRoot: parseOption(argv, "--localai-root"),
			backendId: parseOption(argv, "--backend"),
			targetId: parseOption(argv, "--target"),
		});
		process.stdout.write(`LOCALAI_BACKEND_SOURCE_OK backend=${verified.backend.id} target=${verified.target.id} protocol=${verified.protocolRevision}\n`);
		return;
	}
	if (command === "verify-payload") {
		const result = verifyPayload({
			packageRoot: parseOption(argv, "--package-root"),
			binary: parseOption(argv, "--binary"),
			targetId: parseOption(argv, "--target"),
		});
		process.stdout.write(`LOCALAI_BACKEND_PAYLOAD_OK files=${result.files.length} bytes=${result.bytes}\n`);
		return;
	}
	if (command === "metadata") {
		const config = loadConfig(argv.includes("--config") ? parseOption(argv, "--config") : defaultConfigPath);
		const metadata = buildMetadata({
			config,
			localaiRoot: parseOption(argv, "--localai-root"),
			backendId: parseOption(argv, "--backend"),
			targetId: parseOption(argv, "--target"),
		});
		writeFileSync(parseOption(argv, "--output"), `${JSON.stringify(metadata, null, 2)}\n`);
		process.stdout.write(`LOCALAI_BACKEND_METADATA_OK backend=${metadata.backend} target=${metadata.target.id}\n`);
		return;
	}
	if (command === "join") {
		const config = loadConfig(argv.includes("--config") ? parseOption(argv, "--config") : defaultConfigPath);
		const result = writePublicationBundle({
			config,
			artifactDirectory: parseOption(argv, "--artifact-directory"),
			outputDirectory: parseOption(argv, "--output-directory"),
			repository: parseOption(argv, "--repository"),
		});
		const identity = result.manifest.publication;
		if (argv.includes("--github-output")) {
			const outputPath = parseOption(argv, "--github-output");
			appendFileSync(outputPath, `release_tag=${identity.releaseTag}\n`);
			appendFileSync(outputPath, `pin_fingerprint=${identity.pinFingerprint}\n`);
		}
		process.stdout.write(`LOCALAI_BACKEND_PUBLICATION_BUNDLE_OK release=${identity.releaseTag} artifacts=${result.manifest.artifacts.length}\n`);
		return;
	}
	throw new Error(`unknown command ${command ?? "<missing>"}`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	try {
		main(process.argv.slice(2));
	} catch (error) {
		console.error(`localai-backend-artifact-workflow: ${error.message}`);
		process.exitCode = 1;
	}
}
