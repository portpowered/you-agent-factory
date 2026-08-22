import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { delimiter, dirname, join } from "node:path";
import { env, execPath, platform } from "node:process";
import { spawnSync } from "node:child_process";
import os from "node:os";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const defaultPipelineBanner = "Bare make runs: generate-api, ui-deps, ui-build, build, test, lint.";
const binaryOnlyGuidance = "For only the Go binary, run: make build.";

function makeArgumentPath(path) {
	return path.replaceAll("\\", "/");
}

async function createHarness(t, failMatch = "") {
	const root = await mkdtemp(join(os.tmpdir(), "you-default-pipeline-"));
	const bin = join(root, "bin");
	const logPath = join(root, "tool-events.log");
	await mkdir(bin);
	t.after(() => rm(root, { recursive: true, force: true }));

	const toolScript = join(root, "fake-tool.mjs");
	await writeFile(
		toolScript,
		[
			'import { appendFileSync } from "node:fs";',
			"const [tool, ...args] = process.argv.slice(2);",
			"const command = args.join(\" \" );",
			"appendFileSync(process.env.FAKE_LOG, `${tool}|${command}\\n`);",
			"if (process.env.FAKE_FAIL_MATCH && command.includes(process.env.FAKE_FAIL_MATCH)) process.exit(23);",
		].join("\n") + "\n",
	);

	const toolNames = ["go", "node", "npm", "nested-make"];
	const toolPaths = {};
	for (const tool of toolNames) {
		const filename = platform === "win32" ? `${tool}.cmd` : tool;
		const path = join(bin, filename);
		const body =
			platform === "win32"
				? `@echo off\r\n"${execPath}" "%~dp0..\\fake-tool.mjs" ${tool} %*\r\nexit /b %errorlevel%\r\n`
				: `#!/bin/sh\nexec "$NODE_EXE" "$FAKE_TOOL_SCRIPT" ${tool} "$@"\n`;
		await writeFile(path, body, { mode: 0o755 });
		if (platform !== "win32") await chmod(path, 0o755);
		toolPaths[tool] = path;
	}

	const harnessEnv = {
		...env,
		FAKE_FAIL_MATCH: failMatch,
		FAKE_LOG: logPath,
		FAKE_TOOL_SCRIPT: toolScript,
		NODE_EXE: execPath,
		PATH: `${bin}${delimiter}${env.PATH ?? ""}`,
		...(platform === "win32" ? { PATHEXT: ".CMD;.EXE;.BAT" } : {}),
	};
	return { harnessEnv, logPath, toolPaths };
}

function runMakeTarget(harness, target, extraArgs = []) {
	const args = [
		"--no-print-directory",
		"-f",
		join(repositoryRoot, "Makefile"),
		`GO=${makeArgumentPath(harness.toolPaths.go)}`,
		`NODE=${makeArgumentPath(harness.toolPaths.node)}`,
		`NPM=${makeArgumentPath(harness.toolPaths.npm)}`,
		`MAKE=${makeArgumentPath(harness.toolPaths["nested-make"])}`,
		"BUN_BIN=",
		"YOU_LOGICAL_CPUS=4",
		"YOU_EXPECTED_CONCURRENT_LANES=4",
		"LINT_TARGETS=lint-sentinel",
		...extraArgs,
		target,
	];
	return spawnSync(platform === "win32" ? "make.exe" : "make", args, {
		cwd: repositoryRoot,
		env: harness.harnessEnv,
		encoding: "utf8",
	});
}

function runMake(harness, extraArgs = []) {
	return runMakeTarget(harness, "default", extraArgs);
}

async function toolEvents(logPath) {
	try {
		const contents = await readFile(logPath, "utf8");
		return contents.trim() === "" ? [] : contents.trim().split(/\r?\n/);
	} catch (error) {
		if (error.code === "ENOENT") return [];
		throw error;
	}
}

function phaseForEvent(event) {
	const [tool, command = ""] = event.split("|", 2);
	if (tool === "nested-make") return "nested-make";
	if (tool === "node" && (command.includes("bundle:rest") || command.includes("generate-openapi-types"))) return "generate-api";
	if (tool === "npm" && command.startsWith("exec --package")) return "generate-api";
	if (tool === "go" && command.startsWith("generate ")) return "generate-api";
	if (tool === "npm" && command.startsWith("install")) return "ui-deps";
	if (tool === "npm" && command === "run build") return "ui-build";
	if (tool === "go" && command.startsWith("build ")) return "build";
	if (tool === "go" && command.includes("./cmd/unitlane")) return "test";
	if (tool === "node" && command.startsWith("--test ")) return "test";
	if (tool === "go" && command.includes("./cmd/lintlane")) return "lint";
	return "unknown";
}

function assertDefaultPhaseOrder(events) {
	const phases = events.map(phaseForEvent);
	assert.ok(!phases.includes("unknown"), `unclassified tool events: ${events.join("\\n")}`);
	assert.deepEqual(
		[...new Set(phases)],
		["generate-api", "ui-deps", "ui-build", "build", "test", "lint"],
	);
	assert.equal(phases.filter((phase) => phase === "ui-deps").length, 1);
	assert.equal(phases.filter((phase) => phase === "ui-build").length, 1);
	assert.equal(phases.filter((phase) => phase === "build").length, 1);
	assert.equal(phases.filter((phase) => phase === "test").length, 2);
	assert.equal(phases.filter((phase) => phase === "lint").length, 1);
}

function requireMake(t) {
	const result = spawnSync(platform === "win32" ? "make.exe" : "make", ["--version"], { encoding: "utf8" });
	if (result.error) {
		t.skip(`GNU Make is unavailable: ${result.error.message}`);
		return false;
	}
	return true;
}

test("bare make executes the six default phases once in order", async (t) => {
	if (!requireMake(t)) return;
	const harness = await createHarness(t);
	const result = runMake(harness);
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	assert.ok(result.stdout.includes(defaultPipelineBanner), `missing default pipeline banner: ${result.stdout}`);
	assert.ok(result.stdout.includes(binaryOnlyGuidance), `missing binary-only guidance: ${result.stdout}`);
	const events = await toolEvents(harness.logPath);
	assertDefaultPhaseOrder(events);
});

test("bare make stops later phases when an earlier phase fails", async (t) => {
	if (!requireMake(t)) return;
	const harness = await createHarness(t, "build ");
	const result = runMake(harness);
	assert.notEqual(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const phases = (await toolEvents(harness.logPath)).map(phaseForEvent);
	assert.deepEqual(
		[...new Set(phases)],
		["generate-api", "ui-deps", "ui-build", "build"],
	);
	assert.ok(!phases.includes("test"), `test phase ran after build failure: ${phases.join(", ")}`);
	assert.ok(!phases.includes("lint"), `lint phase ran after build failure: ${phases.join(", ")}`);
});

test("make -n default emits all phases without running tool or nested-make processes", async (t) => {
	if (!requireMake(t)) return;
	const harness = await createHarness(t);
	const result = runMake(harness, ["-n"]);
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	const events = await toolEvents(harness.logPath);
	assert.deepEqual(events, [], `dry run started attributable tools: ${events.join("\\n")}`);
	for (const marker of [
		defaultPipelineBanner,
		binaryOnlyGuidance,
		"bundle:rest",
		"generate-openapi-types",
		"install",
		"run build",
		"cmd/factory",
		"cmd/unitlane",
		"scripts/development-package-workflow.test.mjs",
		"cmd/lintlane",
	]) {
		assert.ok(result.stdout.includes(marker), `dry run omitted ${marker}`);
	}
});

test("local build and install use an overridable VCS build flag", async (t) => {
	if (!requireMake(t)) return;

	const defaultHarness = await createHarness(t);
	for (const target of ["build", "install"]) {
		const result = runMakeTarget(defaultHarness, target, ["GO_BUILD_FLAGS=-a"]);
		assert.equal(result.status, 0, `${target}: ${result.stdout}\n${result.stderr}`);
		const buildEvents = (await toolEvents(defaultHarness.logPath)).filter((event) =>
			event.startsWith("go|build "),
		);
		const buildEvent = buildEvents.at(-1) ?? "";
		assert.match(buildEvent, /^go\|build -a -buildvcs=false -o .+ \.\/cmd\/factory\/$/);
		if (target === "build") {
			const binaryName = platform === "win32" ? "you.exe" : "you";
			assert.match(buildEvent, new RegExp(` -o bin/${binaryName.replace(".", "\\.")} `));
		}
	}

	const stampedHarness = await createHarness(t);
	const stamped = runMakeTarget(stampedHarness, "build", ["GO_LOCAL_BUILD_FLAGS=-buildvcs=true"]);
	assert.equal(stamped.status, 0, `${stamped.stdout}\n${stamped.stderr}`);
	assert.match(
		(await toolEvents(stampedHarness.logPath)).find((event) => event.startsWith("go|build ")) ?? "",
		/-buildvcs=true/,
	);
});
