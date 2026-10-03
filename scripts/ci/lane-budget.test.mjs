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

// Hosted CI passes lane job counts as environment handoffs that a skipped step
// can leave defined but empty, so an explicit undefined removes the inherited
// value on every platform's variable-name casing.
function makeEnv(overrides = {}) {
	const result = { ...env };
	for (const [name, value] of Object.entries(overrides)) {
		for (const key of Object.keys(result)) {
			if (key.toUpperCase() === name.toUpperCase()) delete result[key];
		}
		if (value !== undefined) result[name] = value;
	}
	return result;
}

function runMake(variables, targets = ["print-go-parallelism"], envOverrides = {}) {
	return spawnSync(
		makeCommand,
		["--no-print-directory", "-f", "Makefile", ...variables, ...targets],
		{
			cwd: repositoryRoot,
			env: makeEnv(envOverrides),
			encoding: "utf8",
		},
	);
}

// The dry-run lint recipe is the command CI actually executes, so assert the
// expanded -jobs argument instead of the variable alone.
function expandedLintJobs(stdout) {
	const match = stdout.match(/lintlane\b[^\r\n]*?\s-jobs\s+"(\d+)"/);
	assert.ok(match, `lintlane -jobs argument was not expanded to a positive integer:\n${stdout}`);
	return Number(match[1]);
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

test("explicit functional CI jobs override is shared by discovery and coverage only", (t) => {
	if (!requireMake(t)) return;

	const result = runMake(
		[
			"YOU_LOGICAL_CPUS=8",
			"YOU_EXPECTED_CONCURRENT_LANES=4",
			"FUNCTIONAL_DEFAULT_JOBS=4",
		],
		["-n", "test-unit", "test-functional", "test-functional-coverage"],
	);
	assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
	assert.match(result.stdout, /unitlane -jobs 2/);
	assert.match(result.stdout, /functionallane -jobs 4/);
	assert.match(result.stdout, /gocoveragecheck -suite functional -stream -jobs 4/);
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

test("empty or whitespace LINT_JOBS handoffs fall back to the bounded budget", (t) => {
	if (!requireMake(t)) return;

	const capacity = ["YOU_LOGICAL_CPUS=16", "YOU_EXPECTED_CONCURRENT_LANES=4"];
	// The bounded budget is host dependent, so read the canonical value the same
	// Makefile resolves for an omitted LINT_JOBS instead of restating it here.
	const control = runMake(capacity, ["print-go-parallelism"], { LINT_JOBS: undefined });
	assert.equal(control.status, 0, `${control.stdout}\n${control.stderr}`);
	const budget = outputValue(control.stdout, "GO_LANE_BUDGET");
	const handoffs = [
		{ label: "omitted LINT_JOBS", variables: capacity, envOverrides: { LINT_JOBS: undefined } },
		{ label: "empty LINT_JOBS environment handoff", variables: capacity, envOverrides: { LINT_JOBS: "" } },
		{
			label: "whitespace LINT_JOBS environment handoff",
			variables: capacity,
			envOverrides: { LINT_JOBS: "  " },
		},
		{ label: "empty LINT_JOBS command-line handoff", variables: [...capacity, "LINT_JOBS="] },
	];
	for (const handoff of handoffs) {
		const printed = runMake(handoff.variables, ["print-go-parallelism"], handoff.envOverrides);
		assert.equal(printed.status, 0, `${handoff.label}: ${printed.stdout}\n${printed.stderr}`);
		assert.equal(outputValue(printed.stdout, "GO_LANE_BUDGET"), budget, `${handoff.label}: ${printed.stdout}`);
		assert.equal(outputValue(printed.stdout, "LINT_JOBS"), budget, `${handoff.label}: ${printed.stdout}`);

		const dryRun = runMake(handoff.variables, ["-n", "lint"], handoff.envOverrides);
		assert.equal(dryRun.status, 0, `${handoff.label}: ${dryRun.stdout}\n${dryRun.stderr}`);
		assert.equal(expandedLintJobs(dryRun.stdout), budget, `${handoff.label}: ${dryRun.stdout}`);
	}
});

test("explicit LINT_JOBS overrides are forwarded and non-positive values stay rejected", (t) => {
	if (!requireMake(t)) return;

	const capacity = ["YOU_LOGICAL_CPUS=16", "YOU_EXPECTED_CONCURRENT_LANES=4"];
	const overrides = [
		{ label: "environment override", variables: capacity, envOverrides: { LINT_JOBS: "7" }, jobs: 7 },
		{ label: "command-line override", variables: [...capacity, "LINT_JOBS=3"], jobs: 3 },
	];
	for (const override of overrides) {
		const printed = runMake(override.variables, ["print-go-parallelism"], override.envOverrides);
		assert.equal(printed.status, 0, `${override.label}: ${printed.stdout}\n${printed.stderr}`);
		assert.equal(outputValue(printed.stdout, "LINT_JOBS"), override.jobs, `${override.label}: ${printed.stdout}`);

		const dryRun = runMake(override.variables, ["-n", "lint"], override.envOverrides);
		assert.equal(dryRun.status, 0, `${override.label}: ${dryRun.stdout}\n${dryRun.stderr}`);
		assert.equal(expandedLintJobs(dryRun.stdout), override.jobs, `${override.label}: ${dryRun.stdout}`);
	}

	// An explicit non-positive request must keep reaching lintlane verbatim so
	// its positive-integer parser stays the single rejecting authority.
	for (const invalid of ["LINT_JOBS=0", "LINT_JOBS=many"]) {
		const dryRun = runMake([...capacity, invalid], ["-n", "lint"]);
		assert.equal(dryRun.status, 0, `${invalid}: ${dryRun.stdout}\n${dryRun.stderr}`);
		assert.match(dryRun.stdout, /lintlane\b[^\r\n]*?\s-jobs\s+"[^"]*"/, `${invalid}: ${dryRun.stdout}`);
		assert.ok(
			!/lintlane\b[^\r\n]*?\s-jobs\s+"[1-9]/.test(dryRun.stdout),
			`${invalid} must not be replaced by a budget:\n${dryRun.stdout}`,
		);
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
