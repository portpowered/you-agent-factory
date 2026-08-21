import { appendFileSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { join, normalize, relative, resolve } from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const repositoryRoot = resolve(fileURLToPath(new URL("..", import.meta.url)));
const defaultConfigPath = join(repositoryRoot, ".github", "localai-backend-artifacts.json");
const shaPattern = /^[0-9a-f]{40}$/;
const versionPattern = /^\d+\.\d+(?:\.\d+)?$/;

const expectedBackendIds = ["localai-llamacpp", "localai-whisper", "localai-vibevoice"];
const expectedTargetIds = ["darwin-arm64", "linux-amd64", "windows-amd64"];

const expectedBackendFacts = {
	"localai-llamacpp": {
		sourcePath: "backend/cpp/llama-cpp",
		sourceRepository: "https://github.com/ggerganov/llama.cpp",
		sourcePinVariable: "LLAMA_VERSION",
		makeTarget: "package",
		binary: "grpc-server",
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
	"darwin-arm64": { os: "darwin", architecture: "arm64", runner: "macos-14", buildType: "metal" },
	"linux-amd64": { os: "linux", architecture: "amd64", runner: "ubuntu-24.04", buildType: "cpu" },
	"windows-amd64": { os: "windows", architecture: "amd64", runner: "windows-2022", buildType: "cpu" },
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
	if (expected && (target.os !== expected.os || target.architecture !== expected.architecture || target.runner !== expected.runner || target.buildType !== expected.buildType)) {
		addError(errors, `target ${target.id} does not match its pinned OS, architecture, and build type`);
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
	if (config.localaiRepository !== "https://github.com/mudler/LocalAI.git") {
		addError(errors, "localaiRepository must be the repository-owned LocalAI upstream");
	}
	validateSha(errors, config.localaiCommit, "localaiCommit");
	validateRelativePath(errors, config.protocolPath, "protocolPath");
	if (config.protocolPath !== "backend/backend.proto") addError(errors, "protocolPath must be backend/backend.proto");
	validateSha(errors, config.protocolRevision, "protocolRevision");
	validateSha(errors, config.grpcCommit, "grpcCommit");
	validateSha(errors, config.vcpkgCommit, "vcpkgCommit");

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
		},
		source: {
			repository: config.localaiRepository,
			commit: config.localaiCommit,
			backendRepository: verified.backend.sourceRepository,
			backendCommit: verified.backend.sourceCommit,
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
		if (bytes.length < 20 || bytes[0] !== 0x7f || bytes.toString("ascii", 1, 4) !== "ELF" || bytes[18] !== 0x3e) {
			throw new Error(`${binary} is not an amd64 Linux ELF executable`);
		}
	} else if (targetId === "darwin-arm64") {
		const magic = bytes.length >= 4 ? bytes.readUInt32LE(0) : 0;
		const cpuType = bytes.length >= 8 ? bytes.readUInt32LE(4) : 0;
		const isArm64MachO = magic === 0xfeedfacf && cpuType === 0x0100000c;
		const isUniversalArm64 = magic === 0xbebafeca;
		if (!isArm64MachO && !isUniversalArm64) throw new Error(`${binary} is not an arm64 Darwin Mach-O executable`);
	}
	return { binaryPath, files, bytes: bytes.length };
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
			appendFileSync(outputPath, `localai_commit=${config.localaiCommit}\n`);
			appendFileSync(outputPath, `protocol_revision=${config.protocolRevision}\n`);
			appendFileSync(outputPath, `grpc_commit=${config.grpcCommit}\n`);
			appendFileSync(outputPath, `vcpkg_commit=${config.vcpkgCommit}\n`);
			appendFileSync(outputPath, `go_version=${config.toolchain.goVersion}\n`);
			appendFileSync(outputPath, `cmake_version=${config.toolchain.cmakeVersion}\n`);
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
