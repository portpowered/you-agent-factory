import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const workflow = await readFile(
	new URL("../.github/workflows/development-package.yml", import.meta.url),
	"utf8",
);

function job(name, nextName) {
	const start = workflow.indexOf(`  ${name}:`);
	assert.notEqual(start, -1, `missing ${name} job`);
	const end = nextName ? workflow.indexOf(`  ${nextName}:`, start) : workflow.length;
	return workflow.slice(start, end);
}

test("pull requests retain a read-only dry-run path", () => {
	assert.match(workflow, /^on:\n  pull_request:\s*\n  push:/m);
	assert.match(workflow, /^permissions:\n  contents: read\s*$/m);
	const dryRun = job("dry-run-candidate", "prepare-main-candidate");
	assert.match(dryRun, /if: github\.event_name == 'pull_request'/);
	assert.match(dryRun, /permissions:\n      contents: read/);
	assert.doesNotMatch(dryRun, /id-token\s*:\s*write|npm publish|npm dist-tag/i);
	assert.doesNotMatch(workflow, /NPM_TOKEN|NODE_AUTH_TOKEN/);
});

test("candidate work is blocked on every prerequisite gate", () => {
	for (const name of ["dry-run-candidate", "prepare-main-candidate"]) {
		assert.match(job(name), /needs: validate/);
	}
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

test("workflow hands each reviewed commit to one preserved candidate path", () => {
	const dryRun = job("dry-run-candidate", "prepare-main-candidate");
	assert.match(
		dryRun,
		/--source-commit "\$\{\{ github\.event\.pull_request\.head\.sha \}\}"/,
	);
	assert.match(dryRun, /node scripts\/api-package-pr-dry-run\.mjs/);
	assert.match(dryRun, /if-no-files-found: error/);

	const prepare = job("prepare-main-candidate", "publish-main-candidate");
	assert.match(prepare, /--source-commit "\$\{\{ github\.sha \}\}"/);
	assert.match(prepare, /node scripts\/api-package-candidate\.mjs/);
	assert.match(prepare, /api-development-main-candidate-/);
	assert.equal(
		workflow.match(/node scripts\/api-package-candidate\.mjs/g)?.length,
		1,
	);
});

test("publication is protected-main-only and separately permissioned", () => {
	assert.match(workflow, /push:\n    branches:\n      - main/);
	const publish = job("publish-main-candidate");
	for (const eligibility of [
		"github.event_name == 'push'",
		"github.ref == 'refs/heads/main'",
		"github.repository == 'portpowered/you-agent-factory'",
	]) {
		assert.equal(publish.includes(eligibility), true, eligibility);
	}
	assert.match(publish, /needs:\n      - validate\n      - prepare-main-candidate/);
	assert.match(publish, /environment: development-publishing/);
	assert.match(publish, /runs-on: ubuntu-latest/);
	assert.match(
		publish,
		/permissions:\n      contents: read\n      id-token: write/,
	);
	assert.doesNotMatch(publish, /permissions:\n(?:      (?!contents: read|id-token: write).+\n)+/);
});

test("publish job uses the preserved candidate with trusted npm and exact verification", () => {
	const publish = job("publish-main-candidate");
	assert.match(workflow, /NODE_VERSION: 24/);
	assert.match(workflow, /NPM_VERSION: 11\.15\.0/);
	assert.match(publish, /ref: \$\{\{ github\.sha \}\}/);
	assert.match(publish, /actions\/download-artifact@v4/);
	assert.match(publish, /api-development-main-candidate-/);
	assert.match(publish, /node scripts\/api-package-publish\.mjs/);
	assert.doesNotMatch(publish, /--tag\s+latest|npm dist-tag/i);
});
