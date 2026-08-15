import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const workflow = readFileSync(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");

test("Backend Lint is selected for pull requests and pushes to main", () => {
	assert.match(workflow, /^  pull_request:\s*$/m);
	assert.match(workflow, /^  push:\r?\n    branches: \[main\]\s*$/m);
	assert.match(workflow, /^  backend-lint:\r?\n/m);
	assert.match(
		workflow,
		/if: github\.event_name == 'pull_request' \|\| github\.event_name == 'push'/,
	);
	assert.match(workflow, /ref: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
	assert.match(workflow, /BACKEND_LINT_HEAD_SHA: \$\{\{ github\.event\.pull_request\.head\.sha \|\| github\.sha \}\}/);
});

test("Backend Lint publishes the hosted inventory and feeds Verification Policy", () => {
	assert.match(workflow, /pull-requests: write/);
	assert.match(workflow, /uses: actions\/github-script@v7/);
	assert.match(workflow, /backend-lint-report/);
	assert.match(workflow, /continue-on-error: true/);
	assert.match(workflow, /needs: \[classify,[^\n]*backend-lint/);
	assert.match(workflow, /RUN_BACKEND_LINT: "true"/);
	assert.match(workflow, /BACKEND_LINT_RESULT: \$\{\{ needs\.backend-lint\.result \}\}/);
});
