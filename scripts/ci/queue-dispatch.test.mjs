import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { validateQueueDispatch } from "./queue-dispatch.mjs";

const workflow = readFileSync(new URL("../../.github/workflows/ci.yml", import.meta.url), "utf8");
test("dispatch keeps full queues, mandatory PR proof and maintenance-only main", () => {
	assert.equal(validateQueueDispatch(workflow).status, "pass");
});
test("dispatch guard rejects path-selected queues, missing unit proof and queue cancellation", () => {
	for (const [before, after] of [
		["run_docs_reference: \"true\"", "run_docs_reference: false"],
		["RUN_BACKEND_COVERAGE: \"true\"", "RUN_BACKEND_COVERAGE: false"],
		["cancel-in-progress: ${{ github.event_name == 'pull_request' }}", "cancel-in-progress: true"],
		["if: github.event_name == 'pull_request'\n        run: go run", "if: github.event_name == 'merge_group'\n        run: go run"],
	]) assert.throws(() => validateQueueDispatch(workflow.replace(before, after)), /Dispatch/);
});
