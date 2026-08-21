import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

import {
	loadConfig,
	matrixForConfig,
	validateConfig,
	verifyPayload,
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
