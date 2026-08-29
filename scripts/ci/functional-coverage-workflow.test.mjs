import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import test from "node:test";

const workflowPath = join(process.cwd(), ".github", "workflows", "ci.yml");

function jobSection(workflow, jobName) {
	const match = workflow.match(
		new RegExp(`\\n  ${jobName}:\\n([\\s\\S]*?)(?=\\n  [a-z0-9-]+:\\n|\\s*$)`),
	);
	assert.ok(match, `workflow job is missing: ${jobName}`);
	return match[0];
}

function stepSection(job, marker, nextMarker) {
	const start = job.indexOf(marker);
	assert.ok(start >= 0, `workflow step is missing: ${marker}`);
	const end = job.indexOf(nextMarker, start + marker.length);
	return job.slice(start, end >= 0 ? end : job.length);
}

test("functional coverage uses an isolated cache while unit setup-go remains unchanged", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const unitSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'unit'", "      - uses: actions/setup-go@v5");
	const functionalSetup = stepSection(job, "      - uses: actions/setup-go@v5\n        if: matrix.suite == 'functional'", "      - name: Restore functional coverage Go cache");
	const functionalCache = stepSection(job, "      - name: Restore functional coverage Go cache", "      - uses: actions/setup-node@v4");

	assert.match(unitSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(unitSetup, /cache: true/);
	assert.doesNotMatch(unitSetup, /functional-coverage-|actions\/cache@v4/);

	assert.match(functionalSetup, /go-version: \$\{\{ env\.GO_VERSION \}\}/);
	assert.match(functionalSetup, /cache: false/);
	assert.match(functionalCache, /if: matrix\.suite == 'functional'/);
	assert.match(functionalCache, /id: functional-go-cache/);
	assert.match(functionalCache, /uses: actions\/cache@v4/);
	assert.match(functionalCache, /~\/go\/pkg\/mod/);
	assert.match(functionalCache, /~\/\.cache\/go-build/);
	assert.match(
		functionalCache,
		/key: functional-coverage-\$\{\{ runner\.os \}\}-\$\{\{ runner\.arch \}\}-go-\$\{\{ env\.GO_VERSION \}\}-\$\{\{ hashFiles\('go\.sum'\) \}\}/,
	);
	assert.ok(
		functionalCache.includes(
			"restore-keys: |\n            functional-coverage-${{ runner.os }}-${{ runner.arch }}-go-${{ env.GO_VERSION }}-\n",
		),
		"functional cache restore key must stay in the functional namespace",
	);
	assert.equal((job.match(/uses: actions\/cache@v4/g) ?? []).length, 1);
});

test("functional coverage joins quarantine after concurrent execution and publishes both status paths", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const job = jobSection(workflow, "backend-coverage");
	const supervisorMarker = "      - name: Run Linux functional coverage with concurrent quarantine verification";
	const quarantineUploadMarker = "      - name: Upload Linux quarantine package evidence";
	const supervisor = stepSection(job, supervisorMarker, "      - name: Report functional coverage verdict");
	const quarantineUpload = stepSection(job, quarantineUploadMarker, "      # Ordinary test and coverage-gate failures");

	assert.doesNotMatch(job, /      - name: Verify quarantined package inventory on Linux/);
	assert.match(supervisor, /shell: bash/);
	assert.match(supervisor, /run: bash scripts\/ci\/run-functional-coverage-with-quarantine\.sh/);
	assert.match(supervisor, /FUNCTIONAL_QUARANTINE_EVIDENCE_RUN_URL:/);
	assert.match(supervisor, /FUNCTIONAL_QUARANTINE_EVIDENCE_OUTPUT: \.artifacts\/functional-test-viz\/quarantine-package-evidence\.txt/);
	assert.match(quarantineUpload, /if: always\(\) && matrix\.suite == 'functional'/);
	assert.match(quarantineUpload, /if-no-files-found: error/);
	assert.ok(
		job.indexOf(supervisorMarker) < job.indexOf(quarantineUploadMarker),
		"quarantine evidence must upload after the supervisor has produced it",
	);
	for (const path of [
		".artifacts/functional-test-viz/c09-critical-path/quarantine-status.txt",
		".artifacts/functional-test-viz/c09-critical-path/coverage-status.txt",
		".artifacts/functional-test-viz/c09-critical-path/critical-path-timing.txt",
	]) {
		assert.match(job, new RegExp(path.replaceAll("/", "\\/")));
	}
});
