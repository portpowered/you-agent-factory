import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import test, { describe } from "node:test";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const workflow = readFileSync(join(root, ".github/workflows/ci.yml"), "utf8");

function job(name) {
	const match = workflow.match(new RegExp(`\\n  ${name}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9-]+:\\n|$)`));
	assert.ok(match, `${name} job is missing`);
	return match[1];
}

test("PR architecture previews reuse complete coverage artifacts with read-only permissions", () => {
	const preview = job("backend-visualizations-preview");
	assert.match(preview, /github\.event_name == 'pull_request'/);
	assert.match(preview, /needs\.backend-coverage\.result == 'success'/);
	assert.match(preview, /needs\.verification-policy\.result == 'success'/);
	assert.match(preview, /permissions:\n      contents: read/);
	assert.match(preview, /name: unit-coverage-diagnostics/);
	assert.match(preview, /name: functional-test-diagnostics/);
	assert.match(preview, /BACKEND_VIS_REQUIRE_COVERAGE: "1"/);
	assert.match(preview, /run: make architecture/);
	assert.match(preview, /name: backend-architecture-preview/);
	assert.match(preview, /path: docs\/architecture\/visualizations\//);
	assert.doesNotMatch(preview, /git push|git commit/);
});

// Interpret the shipped Bash blocks; git/gh are controlled external effects.
// These cases do not claim real GitHub authorization or queue admission.
function publicationStep(name) {
	const step = job("backend-visualizations-publish").split(`      - name: ${name}\n`)[1];
	assert.ok(step, `${name} step is missing`);
	return step.split(/\n      - /)[0];
}

function script(step) {
	const body = step.split("        run: |\n")[1];
	assert.ok(body, "Bash run block is missing");
	return body.split("\n").map((line) => line.replace(/^          /, "")).join("\n");
}

const runFile = promisify(execFile);
const bash = process.platform === "win32" ? "C:/Program Files/Git/bin/bash.exe" : "bash";
const prURL = "https://github.com/example/factory/pull/42";
const effects = String.raw`
record() { printf '%s\t' "$@" >> "$EFFECT_LOG"; printf '\n' >> "$EFFECT_LOG"; }
git() {
  record git "$@"
  case "$1" in
    add|switch|config|commit) return 0 ;;
    diff) [[ "$CANDIDATE_CHANGED" == false ]]; return ;;
    push)
      if [[ "$FAIL_AT" == push ]]; then echo 'controlled push failure' >&2; return 41; fi
      return 0 ;;
    *) echo 'unexpected git operation' >&2; return 99 ;;
  esac
}
gh() {
  record gh "$@"
  case "$1 $2" in
    'auth setup-git') return 0 ;;
    'pr create')
      if [[ "$FAIL_AT" == create ]]; then echo 'controlled create failure' >&2; return 42; fi
      printf '%s\n' "$PR_URL" ;;
    'pr merge')
      if [[ "$FAIL_AT" == merge ]]; then echo 'controlled merge failure' >&2; return 43; fi ;;
    *) echo 'unexpected gh operation' >&2; return 99 ;;
  esac
}
readonly -f git gh
`;

async function runScript(body, overrides = {}) {
	const directory = mkdtempSync(join(tmpdir(), "architecture-publication-"));
	const shellPath = (path) => path.replaceAll("\\", "/");
	const log = join(directory, "effects");
	const output = join(directory, "output");
	writeFileSync(log, "");
	writeFileSync(output, "");
	try {
		let result;
		try {
			result = await runFile(bash, ["--noprofile", "--norc", "-eo", "pipefail", "-c", effects + body], {
				cwd: directory,
				timeout: 60000,
				env: {
					// Do not inherit BASH_ENV, tokens, or developer git/gh configuration.
					PATH: process.platform === "win32" ? "C:/Program Files/Git/usr/bin" : "/usr/bin:/bin",
					SystemRoot: process.env.SystemRoot,
					GH_TOKEN: "fake-installation-token",
					GITHUB_OUTPUT: shellPath(output),
					RUNNER_TEMP: shellPath(directory),
					EFFECT_LOG: shellPath(log),
					GITHUB_RUN_ID: "123",
					GITHUB_RUN_ATTEMPT: "2",
					GITHUB_SERVER_URL: "https://github.com",
					GITHUB_REPOSITORY: "example/factory",
					GITHUB_SHA: "abcdef",
					PR_URL: prURL,
					CANDIDATE_CHANGED: "true",
					FAIL_AT: "",
					...overrides,
				},
			});
			result.code = 0;
		} catch (error) {
			assert.equal(typeof error.code, "number", `Bash could not execute: ${error.message}`);
			result = error;
		}
		return {
			code: result.code, stdout: result.stdout, stderr: result.stderr,
			calls: readFileSync(log, "utf8").split("\n").filter(Boolean).map((line) => line.split("\t").slice(0, -1)),
			output: readFileSync(output, "utf8"),
			body: result.code === 0 && body.includes("gh pr create")
				? readFileSync(join(directory, "backend-architecture-pr.md"), "utf8") : "",
		};
	} finally {
		rmSync(directory, { recursive: true, force: true });
	}
}

const candidate = script(publicationStep("Check for changed architecture pages"));
const publication = script(publicationStep("Propose and auto-merge measured pages"));

describe("executed architecture publication scripts", { concurrency: true }, () => {
	test("CASE-1: changed pages create one PR and request queue-managed auto-merge", { concurrency: true }, async () => {
		const detected = await runScript(candidate);
		assert.equal(detected.code, 0);
		assert.equal(detected.output, "changed=true\n");
		const result = await runScript(publication);
		assert.equal(result.code, 0, result.stderr);
		const remoteCalls = result.calls.filter(([tool, operation]) => tool === "gh" || operation === "push");
		assert.deepEqual(remoteCalls, [
			["gh", "auth", "setup-git", "--hostname", "github.com"],
			["git", "push", "origin", "HEAD:automation/backend-architecture-123-2"],
			["gh", "pr", "create", "--base", "main", "--head", "automation/backend-architecture-123-2",
				"--title", "docs: refresh backend architecture visualizations", "--body-file",
				result.calls.find((call) => call[0] === "gh" && call[2] === "create").at(-1)],
			["gh", "pr", "merge", prURL, "--auto"],
		]);
		assert.match(result.body, /https:\/\/github.com\/example\/factory\/actions\/runs\/123/);
	});

	test("CASE-2: unchanged pages exclude publication", { concurrency: true }, async () => {
		const result = await runScript(candidate, { CANDIDATE_CHANGED: "false" });
		assert.equal(result.code, 0, result.stderr);
		assert.equal(result.output, "changed=false\n");
		assert.match(result.stdout, /Backend architecture pages are current\./);
		assert.deepEqual(result.calls, [["git", "add", "-A", "docs/architecture/visualizations/"], ["git", "diff", "--cached", "--quiet"]]);
		assert.match(publicationStep("Propose and auto-merge measured pages"), /if: steps\.candidate\.outputs\.changed == 'true'/);
	});

	test("CASE-3: missing credential fails before external effects", { concurrency: true }, async () => {
		const result = await runScript(publication, { GH_TOKEN: "" });
		assert.equal(result.code, 1);
		assert.match(result.stderr, /Missing bot credential/);
		assert.deepEqual(result.calls, []);
	});

	for (const [id, failure, code] of [[4, "push", 41], [5, "create", 42], [6, "merge", 43]]) {
		test(`CASE-${id}: ${failure} failure propagates without later effects`, { concurrency: true }, async () => {
			const result = await runScript(publication, { FAIL_AT: failure });
			assert.equal(result.code, code);
			assert.match(result.stderr, new RegExp(`controlled ${failure} failure`));
			const last = result.calls.at(-1);
			assert.equal(failure === "push" ? last[1] : last[2], failure);
			assert.equal(result.calls.filter((call) => call[0] === "gh" && call[2] === "create").length, failure === "push" ? 0 : 1);
			assert.equal(result.calls.filter((call) => call[0] === "gh" && call[2] === "merge").length, failure === "merge" ? 1 : 0);
			if (failure === "merge") assert.deepEqual(last, ["gh", "pr", "merge", prURL, "--auto"]);
			assert.equal(result.stdout, "");
		});
	}
});

test("successful main CI proposes measured pages through a bot pull request", () => {
	const publish = job("backend-visualizations-publish");
	assert.match(publish, /github\.event_name == 'push' && github\.ref == 'refs\/heads\/main'/);
	assert.match(publish, /needs\.main-evidence\.result == 'success'/);
	assert.match(publish, /needs: main-evidence/);
	assert.match(publish, /permissions:\n      contents: read/);
	assert.match(publish, /fetch-depth: 2/);
	assert.match(publish, /persist-credentials: false/);
	assert.match(publish, /git diff --quiet HEAD\^ HEAD -- \. ':\(exclude\)docs\/architecture\/visualizations\/\*\*'/);
	assert.match(publish, /BACKEND_VIS_SOURCE_COMMIT: \$\{\{ github\.sha \}\}/);
	assert.match(publish, /BACKEND_VIS_REQUIRE_COVERAGE: "1"/);
	assert.match(publish, /run: make architecture/);
	assert.match(publish, /git add -A docs\/architecture\/visualizations\//);
	assert.match(publish, /git diff --cached --quiet/);
	assert.match(publish, /uses: actions\/create-github-app-token@v2/);
	assert.match(publish, /gh auth setup-git --hostname github\.com/);
	assert.match(publish, /git push origin "HEAD:\$bot_branch"/);
	assert.match(publish, /gh pr create --base main --head "\$bot_branch"/);
	assert.match(publish, /gh pr merge "\$pr_url" --auto\s*$/m);
	assert.doesNotMatch(publish, /--delete-branch/);
	assert.doesNotMatch(publish, /git push origin HEAD:main/);
});
