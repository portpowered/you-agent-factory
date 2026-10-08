import assert from "node:assert/strict";
import test from "node:test";

import { selectPublishedBackendConformance } from "./published-backend-conformance-workflow.mjs";

test("scheduled and manual events select the live published-backend cell", () => {
	for (const eventName of ["schedule", "workflow_dispatch"]) {
		const selection = selectPublishedBackendConformance({ eventName });
		assert.equal(selection.selected, true, eventName);
		assert.match(selection.reason, /live cell/);
	}
});

test("pull requests do not select live published-backend requests", () => {
	for (const eventName of ["pull_request", "merge_group", "push", "release", "workflow_call", undefined]) {
		const selection = selectPublishedBackendConformance({ eventName });
		assert.equal(selection.selected, false, eventName);
		assert.match(selection.reason, /do not run live/);
	}
});
