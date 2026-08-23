import assert from "node:assert/strict";
import test from "node:test";

import { selectBackendIntegration } from "./backend-integration-workflow.mjs";

test("selects pull requests and protected-main pushes", () => {
	assert.deepEqual(
		selectBackendIntegration({
			eventName: "pull_request",
			ref: "refs/pull/42/merge",
		}),
		{ selected: true, error: "" },
	);
	assert.deepEqual(
		selectBackendIntegration({
			eventName: "push",
			ref: "refs/heads/main",
		}),
		{ selected: true, error: "" },
	);
});

test("does not select unrelated events or branch pushes", () => {
	for (const input of [
		{ eventName: "push", ref: "refs/heads/feature" },
		{ eventName: "workflow_dispatch", ref: "refs/heads/main" },
		{ eventName: "", ref: "" },
	]) {
		assert.deepEqual(selectBackendIntegration(input), { selected: false, error: "" });
	}
});
