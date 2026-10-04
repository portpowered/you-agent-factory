import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

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

test("successful main CI proposes measured pages through a bot pull request", () => {
	const publish = job("backend-visualizations-publish");
	assert.match(publish, /github\.event_name == 'push' && github\.ref == 'refs\/heads\/main'/);
	assert.match(publish, /needs\.backend-coverage\.result == 'success'/);
	assert.match(publish, /needs\.verification-policy\.result == 'success'/);
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
