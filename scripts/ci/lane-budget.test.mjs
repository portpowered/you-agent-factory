import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { env, platform } from "node:process";
import { spawnSync } from "node:child_process";
import test from "node:test";

const repositoryRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const makeCommand = platform === "win32" ? "make.exe" : "make";

function requireMake(t) {
	const result = spawnSync(makeCommand, ["--version"], { encoding: "utf8" });
	if (result.error) {
		t.skip(`GNU Make is unavailable: ${result.error.message}`);
		return false;
	}
	return true;
}

function runMake(variables, targets = ["print-go-parallelism"]) {
	return spawnSync(
		makeCommand,
		["--no-print-directory", "-f", "Makefile", ...variables, ...targets],
		{
			cwd: repositoryRoot,
			env,
			encoding: "utf8",
		},
	);
}

function outputValue(stdout, name) {
	const match = stdout.match(new RegExp(`(?:^|\\r?\\n)"?${name}=(\\d+)"?(?:\\r?\\n|$)`));
	assert.ok(match, `${name} was not a positive integer in output:\n${stdout}`);
	return Number(match[1]);
}

function assertBudgetOutput(result, expected) {
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	for (const name of ["GO_LANE_BUDGET", "FUNCTIONAL_DEFAULT_JOBS", "UNIT_DEFAULT_JOBS", "LINT_JOBS"]) {
		assert.equal(outputValue(result.stdout, name), expected, `${name} output:\n${result.stdout}`);
	}
}

function installedPosixShell() {
	const candidates = [
		env.SHELL,
		env.ProgramFiles ? join(env.ProgramFiles, "Git", "bin", "sh.exe") : "",
		env.ProgramFiles ? join(env.ProgramFiles, "Git", "usr", "bin", "sh.exe") : "",
	];
	return candidates.find((candidate) => candidate && existsSync(candidate));
}

test("production Make computation yields the bounded numeric budget", (t) => {
	if (!requireMake(t)) return;

	const result = runMake(["YOU_LOGICAL_CPUS=16", "YOU_EXPECTED_CONCURRENT_LANES=4"]);
	assertBudgetOutput(result, 4);
});

test("explicit functional CI jobs keep discovery and instrumented coverage separate", (t) => {
	if (!requireMake(t)) return;

	const result = runMake(
		[
			"YOU_LOGICAL_CPUS=8",
			"YOU_EXPECTED_CONCURRENT_LANES=4",
			"FUNCTIONAL_DEFAULT_JOBS=4",
			"FUNCTIONAL_TEST_JOBS=8",
		],
		["-n", "test-unit", "test-functional", "test-functional-coverage"],
	);
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	assert.match(result.stdout, /unitlane -jobs 2/);
	assert.match(result.stdout, /functionallane -jobs 4/);
	assert.match(result.stdout, /gocoveragecheck -suite functional -stream -jobs 4 -test-jobs 8/);
	assert.doesNotMatch(result.stdout, /functionallane -jobs 4 .*test-jobs/);
});

test("functional coverage without the CI test-window override keeps the existing command", (t) => {
	if (!requireMake(t)) return;

	const result = runMake(
		[
			"YOU_LOGICAL_CPUS=8",
			"YOU_EXPECTED_CONCURRENT_LANES=4",
			"FUNCTIONAL_DEFAULT_JOBS=4",
		],
		["-n", "test-functional-coverage"],
	);
	assert.equal(result.status, 0);
	assert.match(result.stdout, /gocoveragecheck -suite functional -stream -jobs 4(?! -test-jobs)/);
	assert.doesNotMatch(result.stdout, /-test-jobs/);
});

test("corrupted production result warns, falls back, and reaches numeric job flags", (t) => {
	if (!requireMake(t)) return;

	for (const computedValue of ["0", "", "Windows banner text", "4x"]) {
		const result = runMake(
			[
				"YOU_LOGICAL_CPUS=16",
				"YOU_EXPECTED_CONCURRENT_LANES=4",
				`GO_LANE_BUDGET_COMPUTED=${computedValue}`,
			],
			["-n", "test-unit", "test-functional", "lint"],
		);
		assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
		assert.ok(
			result.stderr.includes(`GO_LANE_BUDGET received invalid computed value '${computedValue}'; using 2`),
			`missing invalid-value warning for ${JSON.stringify(computedValue)}:\n${result.stderr}`,
		);
		assert.match(result.stdout, /unitlane -jobs 2/);
		assert.match(result.stdout, /functionallane -jobs 2/);
		assert.match(result.stdout, /lintlane -make .* -jobs "2"/);
	}
});

test("Windows Make uses numeric arithmetic through sh-compatible and cmd shells", (t) => {
	if (platform !== "win32") {
		t.skip("Windows shell-family coverage runs on Windows CI");
		return;
	}
	if (!requireMake(t)) return;

	const common = ["OS=Windows_NT", "NUMBER_OF_PROCESSORS=16", "YOU_EXPECTED_CONCURRENT_LANES=4"];
	const posixShell = installedPosixShell();
	if (!posixShell) {
		t.skip("a Git sh.exe installation is unavailable");
		return;
	}
	const shells = [`SHELL=${posixShell}`, "SHELL=cmd.exe"];
	for (const shell of shells) {
		const result = runMake([...common, shell]);
		assertBudgetOutput(result, 4);
	}
});
