import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { spawn, spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

const repositoryRoot = process.cwd();
const supervisorPath = join(
	repositoryRoot,
	"scripts",
	"ci",
	"run-functional-coverage-with-quarantine.sh",
);
const bashCommand = process.env.BASH_BIN || "bash";

function pathForBash(path) {
	if (process.platform !== "win32") return path;
	const windowsPath = path.match(/^([A-Za-z]):[\\/](.*)$/);
	if (!windowsPath) return path.replaceAll("\\", "/");
	return `/mnt/${windowsPath[1].toLowerCase()}/${windowsPath[2].replaceAll("\\", "/")}`;
}

const bashSupervisorPath = pathForBash(supervisorPath);
const bashProbe = spawnSync(bashCommand, ["-c", "exit 0"], {
	encoding: "utf8",
	windowsHide: true,
});
const bashAvailable = !bashProbe.error && bashProbe.status === 0;

function shellQuote(value) {
	return `'${value.replaceAll("'", "'\\\"'\\\"'")}'`;
}

function fakeCommand({ pidPath, startedPath, waitPath = "", delay = "0", exit = 0 }) {
	return `#!/usr/bin/env bash
set -u
pid_file=${shellQuote(pathForBash(pidPath))}
started_file=${shellQuote(pathForBash(startedPath))}
printf '%s\\n' "$$" > "$pid_file"
printf '%s\\n' started > "$started_file"
wait_file=${shellQuote(pathForBash(waitPath))}
if [[ -n "$wait_file" ]]; then
  # This short loop keeps a real child alive for the cancellation case; the
  # test synchronizes on started_file before signalling the supervisor.
  while [[ ! -f "$wait_file" ]]; do
    sleep 0.01
  done
fi
delay=${shellQuote(delay)}
if [[ "$delay" != "0" ]]; then
  sleep "$delay"
fi
exit ${exit}
`;
}

function statusPath(directory, name) {
	return join(directory, `${name}-status.txt`);
}

function timingPath(directory) {
	return join(directory, "critical-path-timing.txt");
}

function readStatus(directory, name) {
	return Number.parseInt(readFileSync(statusPath(directory, name), "utf8").trim(), 10);
}

function supervisorArguments(directory, commands) {
	return [
		bashSupervisorPath,
		"--status-dir",
		pathForBash(directory),
		"--quarantine-command",
		pathForBash(commands.quarantine),
		"--coverage-command",
		pathForBash(commands.coverage),
	];
}

function runSupervisor(directory, commands) {
	return spawnSync(bashCommand, supervisorArguments(directory, commands), {
		cwd: repositoryRoot,
		env: {
			...process.env,
		},
		encoding: "utf8",
		windowsHide: true,
	});
}

function outputOf(result) {
	return `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
}

async function waitForFile(path, child) {
	const deadline = Date.now() + 5000;
	while (!existsSync(path) && Date.now() < deadline && child.exitCode === null) {
		// The marker is the deterministic synchronization edge for this process
		// test; a fixed sleep would race the signal against child startup.
		await new Promise((resolve) => setTimeout(resolve, 10));
	}
	assert.equal(existsSync(path), true, `child did not create ${path}`);
}

function waitForExit(child) {
	return new Promise((resolve, reject) => {
		child.once("error", reject);
		child.once("close", (code, signal) => resolve({ code, signal }));
	});
}

async function createCommands(directory, options = {}) {
	const quarantine = join(directory, "quarantine.sh");
	const coverage = join(directory, "coverage.sh");
	const quarantinePidPath = join(directory, "quarantine-pid");
	const coveragePidPath = join(directory, "coverage-pid");
	await writeFile(
		quarantine,
		fakeCommand({
			pidPath: quarantinePidPath,
			startedPath: join(directory, "quarantine-started"),
			waitPath: options.quarantineWaitPath,
			delay: options.quarantineDelay,
			exit: options.quarantineExit,
		}),
		{ mode: 0o755 },
	);
	await writeFile(
		coverage,
		fakeCommand({
			pidPath: coveragePidPath,
			startedPath: join(directory, "coverage-started"),
			waitPath: options.coverageWaitPath,
			delay: options.coverageDelay,
			exit: options.coverageExit,
		}),
		{ mode: 0o755 },
	);
	return { quarantine, coverage, quarantinePidPath, coveragePidPath };
}

test(
	"functional coverage supervisor joins independent outcomes and cleans up cancellation",
	{ skip: !bashAvailable, skipReason: "bash is required for the CI supervisor test" },
	async (t) => {
		const root = await mkdtemp(join(tmpdir(), "functional-coverage-supervisor-"));
		t.after(() => rm(root, { recursive: true, force: true }));

		async function scenario(name, options = {}) {
			const directory = join(root, name);
			await mkdir(directory);
			const commands = await createCommands(directory, options);
			return {
				directory,
				result: runSupervisor(directory, commands),
			};
		}

		const success = await scenario("success", {});
		assert.equal(success.result.status, 0);
		assert.equal(readStatus(success.directory, "quarantine"), 0);
		assert.equal(readStatus(success.directory, "coverage"), 0);

		const quarantineFailure = await scenario("quarantine-failure", { quarantineExit: 7 });
		assert.equal(quarantineFailure.result.status, 1);
		assert.equal(readStatus(quarantineFailure.directory, "quarantine"), 7);
		assert.equal(readStatus(quarantineFailure.directory, "coverage"), 0);
		assert.match(outputOf(quarantineFailure.result), /Functional quarantine inventory validation failed/);

		const coverageFailure = await scenario("coverage-failure", { coverageExit: 9 });
		assert.equal(coverageFailure.result.status, 1);
		assert.equal(readStatus(coverageFailure.directory, "quarantine"), 0);
		assert.equal(readStatus(coverageFailure.directory, "coverage"), 9);
		assert.match(outputOf(coverageFailure.result), /Backend functional coverage failed/);

		const dualFailure = await scenario("dual-failure", { quarantineExit: 7, coverageExit: 9 });
		assert.equal(dualFailure.result.status, 1);
		assert.equal(readStatus(dualFailure.directory, "quarantine"), 7);
		assert.equal(readStatus(dualFailure.directory, "coverage"), 9);
		assert.match(outputOf(dualFailure.result), /Functional quarantine inventory validation failed/);
		assert.match(outputOf(dualFailure.result), /Backend functional coverage failed/);

		const inverted = await scenario("completion-order-inversion", {
			quarantineDelay: "0.15",
			coverageDelay: "0.01",
		});
		assert.equal(inverted.result.status, 0);
		const invertedTiming = readFileSync(timingPath(inverted.directory), "utf8");
		assert.ok(invertedTiming.indexOf("coverage-start=") < invertedTiming.indexOf("quarantine-end="));
		assert.ok(invertedTiming.indexOf("coverage-end=") < invertedTiming.indexOf("quarantine-end="));
		if (process.platform === "win32") {
			// The supported CI executor is Linux; the Windows WSL launcher does not
			// propagate SIGTERM to a spawned Linux process group reliably.
			return;
		}

		const cancellationDirectory = join(root, "cancellation");
		await mkdir(cancellationDirectory);
		const cancellationCommands = await createCommands(cancellationDirectory, {
			quarantineWaitPath: join(cancellationDirectory, "release-quarantine"),
			coverageWaitPath: join(cancellationDirectory, "release-coverage"),
		});
		const child = spawn(bashCommand, supervisorArguments(cancellationDirectory, cancellationCommands), {
			cwd: repositoryRoot,
			stdio: ["ignore", "pipe", "pipe"],
			windowsHide: true,
		});
		const childOutput = [];
		child.stdout.on("data", (chunk) => childOutput.push(chunk.toString()));
		child.stderr.on("data", (chunk) => childOutput.push(chunk.toString()));
		await Promise.all([
			waitForFile(join(cancellationDirectory, "quarantine-started"), child),
			waitForFile(join(cancellationDirectory, "coverage-started"), child),
		]);
		assert.equal(child.kill("SIGTERM"), true);
		const cancellationResult = await waitForExit(child);
		assert.equal(cancellationResult.code, 130);
		assert.match(childOutput.join(""), /cancelled by TERM/);
		assert.notEqual(readStatus(cancellationDirectory, "quarantine"), 0);
		assert.notEqual(readStatus(cancellationDirectory, "coverage"), 0);

		if (process.platform !== "win32") {
			for (const [name, pidPath] of [
				["quarantine", cancellationCommands.quarantinePidPath],
				["coverage", cancellationCommands.coveragePidPath],
			]) {
				const pid = Number.parseInt(readFileSync(pidPath, "utf8").trim(), 10);
				const probe = spawnSync("kill", ["-0", String(pid)]);
				assert.notEqual(probe.status, 0, `${name} child ${pid} remained after cancellation`);
			}
		}
	},
);
