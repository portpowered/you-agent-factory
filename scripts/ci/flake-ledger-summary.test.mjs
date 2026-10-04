import assert from "node:assert/strict";
import test from "node:test";
import { parseFlakeLedger, renderFlakeAnnotations, renderFlakeSummary } from "./flake-ledger-summary.mjs";

const recovered = {
	schemaVersion: 1,
	headSha: "0123456789abcdef0123",
	outcome: "flake-recovered",
	reason: "every retried test passed",
	retryLimit: 5,
	entries: [
		{
			package: "example/tests/functional/runtime_api",
			test: "TestFlaky",
			retryOutcome: "pass",
			firstFailureExcerpt: "got | want\nsecond line",
		},
	],
};

test("parseFlakeLedger rejects malformed ledgers", () => {
	assert.throws(() => parseFlakeLedger("{"), /not valid JSON/);
	assert.throws(() => parseFlakeLedger(JSON.stringify({ ...recovered, schemaVersion: 2 })), /schemaVersion/);
	assert.throws(() => parseFlakeLedger(JSON.stringify({ ...recovered, outcome: "ok" })), /outcome/);
	assert.throws(() => parseFlakeLedger(JSON.stringify({ ...recovered, entries: null })), /entries/);
	assert.equal(parseFlakeLedger(JSON.stringify(recovered)).entries.length, 1);
});

test("a recovered flake renders a table row and a warning annotation", () => {
	const summary = renderFlakeSummary(recovered, { runUrl: "https://example.test/run/1" });
	assert.match(summary, /FLAKE RECORDED/);
	assert.match(summary, /`0123456789ab`/);
	assert.match(summary, /\| `TestFlaky` \| `example\/tests\/functional\/runtime_api` \| pass \| got \\| want second line \|/);
	assert.match(summary, /https:\/\/example\.test\/run\/1/);
	assert.deepEqual(renderFlakeAnnotations(recovered), [
		"::warning title=Flaky functional test::example/tests/functional/runtime_api TestFlaky failed once and passed on same-head retry",
	]);
});

test("a persistent failure or skipped retry is not announced as a flake", () => {
	const failed = { ...recovered, outcome: "failed-after-retry", entries: [{ ...recovered.entries[0], retryOutcome: "fail" }] };
	assert.match(renderFlakeSummary(failed), /Failed after retry/);
	assert.deepEqual(renderFlakeAnnotations(failed), []);
	const skipped = { ...recovered, outcome: "not-retried", reason: "7 failing tests exceed the retry limit of 5", entries: [] };
	const summary = renderFlakeSummary(skipped);
	assert.match(summary, /Not retried\.\*\* 7 failing tests exceed/);
	assert.doesNotMatch(summary, /FLAKE RECORDED/);
});
