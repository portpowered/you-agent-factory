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

test("selected Backend Conformance runs offline then live and fails closed through policy", () => {
	const workflow = readFileSync(workflowPath, "utf8");
	const conformanceJob = jobSection(workflow, "backend-conformance");
	const policyJob = jobSection(workflow, "verification-policy");
	const offlineStep = conformanceJob.indexOf("run: make test-backend-conformance");
	const liveStep = conformanceJob.indexOf("run: make test-backend-conformance-live");

	assert.match(
		conformanceJob,
		/if: always\(\) && needs\.classify\.outputs\.run_backend_conformance != 'false'/,
	);
	assert.match(conformanceJob, /timeout-minutes: 10/);
	assert.ok(offlineStep >= 0, "offline backend conformance step is missing");
	assert.ok(liveStep > offlineStep, "live validation must follow offline conformance");
	assert.doesNotMatch(conformanceJob, /continue-on-error:\s*true/);

	assert.match(policyJob, /needs: \[[^\]]*\bbackend-conformance\b[^\]]*\]/s);
	assert.match(
		policyJob,
		/BACKEND_CONFORMANCE_RESULT: \$\{\{ needs\.backend-conformance\.result \}\}/,
	);
});
