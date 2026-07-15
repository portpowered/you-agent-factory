import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflow = await readFile(
	new URL("../.github/workflows/development-package.yml", import.meta.url),
	"utf8",
);

test("development package workflow grants pull requests read-only authority", () => {
	assert.match(workflow, /^on:\n  pull_request:\s*$/m);
	assert.match(workflow, /^permissions:\n  contents: read\s*$/m);
	assert.doesNotMatch(workflow, /id-token\s*:\s*write/);
	assert.doesNotMatch(
		workflow,
		/NPM_TOKEN|NODE_AUTH_TOKEN|registry-url|npm publish|npm dist-tag/i,
	);
});

test("candidate dry run is blocked on every prerequisite gate", () => {
	assert.match(
		workflow,
		/dry-run-candidate:\n(?:.|\n)*?    needs: validate\n/,
	);
	for (const command of [
		"make typecheck",
		"make lint",
		"make test",
		"make contracts-smoke",
		"make api-smoke",
		"make api-package-pack-smoke",
	]) {
		assert.equal(workflow.includes(`run: ${command}`), true, command);
	}
});

test("workflow hands the reviewed commit to one preserved candidate smoke path", () => {
	assert.match(
		workflow,
		/--source-commit "\$\{\{ github\.event\.pull_request\.head\.sha \}\}"/,
	);
	assert.match(workflow, /node scripts\/api-package-pr-dry-run\.mjs/);
	assert.match(
		workflow,
		/--output-directory \.artifacts\/api-development-candidate\/package/,
	);
	assert.match(workflow, /path: \.artifacts\/api-development-candidate\//);
	assert.match(workflow, /if-no-files-found: error/);
	assert.equal(
		workflow.match(/node scripts\/api-package-pr-dry-run\.mjs/g)?.length,
		1,
	);
});
