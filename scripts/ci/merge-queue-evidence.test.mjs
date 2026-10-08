import assert from "node:assert/strict";
import test from "node:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveQueueEvidence, forwardQueueEvidence, REQUIRED_ARTIFACTS } from "./merge-queue-evidence.mjs";
import { warmMainDependencyCaches } from "./main-dependency-cache.mjs";

const mainSha = "a".repeat(40), queueSha = "b".repeat(40), tree = "c".repeat(40);
const repository = "owner/repo";
const run = { id: 42, run_attempt: 2, event: "merge_group", status: "completed", conclusion: "success", head_sha: queueSha, head_branch: "gh-readonly-queue/main/pr-1-test", head_repository: { full_name: repository }, html_url: "https://github.com/owner/repo/actions/runs/42" };
function fixture({ queueTree = tree, runs = [run], artifacts = REQUIRED_ARTIFACTS.map((name) => ({ name, expired: false })) } = {}) {
	return async (path) => {
		if (path.endsWith(`/git/commits/${mainSha}`)) return { tree: { sha: tree } };
		if (path.includes("/git/commits/")) return { tree: { sha: queueTree } };
		if (path.includes("/artifacts?")) return { artifacts, total_count: artifacts.length };
		assert.match(path, /workflows\/ci\.yml\/runs\?event=merge_group&status=success/);
		return { workflow_runs: runs };
	};
}
const options = (overrides = {}) => ({ repository, mainSha, api: fixture(), ...overrides });

test("squash delivery forwards exact-tree artifact bytes and queue provenance", async () => {
	const directory = mkdtempSync(join(tmpdir(), "queue-evidence-"));
	try {
		const source = await forwardQueueEvidence(options({ outputDirectory: directory }), async (selected, name, destination) => {
			assert.equal(selected.runId, 42);
			writeFileSync(join(destination, "evidence.txt"), `original ${name}\n`);
		});
		assert.equal(source.tree, tree);
		assert.notEqual(source.mainSha, source.queueSha);
		for (const name of REQUIRED_ARTIFACTS) {
			assert.equal(readFileSync(join(directory, name, "evidence.txt"), "utf8"), `original ${name}\n`);
			assert.deepEqual(JSON.parse(readFileSync(join(directory, name, "queue-evidence-provenance.json"))), source);
		}
	} finally { rmSync(directory, { recursive: true, force: true }); }
});

test("wrong tree, failed queue, foreign repo, wrong target and push evidence are refused", async () => {
	await assert.rejects(resolveQueueEvidence(options({ api: fixture({ queueTree: "d".repeat(40) }) })), /exact tree/);
	for (const change of [{ conclusion: "failure" }, { event: "push" }, { status: "in_progress" }, { head_repository: { full_name: "other/repo" } }, { head_branch: "gh-readonly-queue/other/pr-1" }]) {
		await assert.rejects(resolveQueueEvidence(options({ api: fixture({ runs: [{ ...run, ...change }] }) })), /exact tree/);
	}
});

test("missing, duplicate and expired artifacts and API failures propagate", async () => {
	for (const artifacts of [[], REQUIRED_ARTIFACTS.map((name) => ({ name, expired: true })), REQUIRED_ARTIFACTS.flatMap((name) => [{ name }, { name }])]) {
		await assert.rejects(resolveQueueEvidence(options({ api: fixture({ artifacts }) })), /requires one unexpired/);
	}
	await assert.rejects(resolveQueueEvidence(options({ api: async () => { throw new Error("403 denied"); } })), /403 denied/);
	await assert.rejects(resolveQueueEvidence(options({ mainSha: "bad" })), /complete main commit/);
});

test("download failure refuses main evidence publication", async () => {
	const directory = mkdtempSync(join(tmpdir(), "queue-download-"));
	try {
		await assert.rejects(forwardQueueEvidence(options({ outputDirectory: directory }), async () => { throw new Error("download denied"); }), /download denied/);
	} finally { rmSync(directory, { recursive: true, force: true }); }
});

test("queue search follows pagination and refuses missing main trees or truncated artifact lists", async () => {
	const calls = [];
	const initial = fixture();
	const api = async (path) => {
		calls.push(path);
		if (path.endsWith("&page=1")) return { workflow_runs: Array.from({ length: 100 }, () => ({ ...run, event: "pull_request" })) };
		return initial(path);
	};
	assert.equal((await resolveQueueEvidence(options({ api }))).runId, 42);
	assert.ok(calls.some((path) => path.endsWith("page=2")));
	await assert.rejects(resolveQueueEvidence(options({ api: async () => ({}) })), /Main commit tree is missing/);
	await assert.rejects(resolveQueueEvidence(options({ api: async (path) => path.includes("/artifacts?") ? { artifacts: [], total_count: 101 } : initial(path) })), /truncated artifact/);
});

test("cache hits do nothing; either dependency-key miss builds only dependencies", () => {
	for (const [functionalHit, unitHit] of [["true", "true"], ["false", "true"], ["true", "false"], ["", ""]]) {
		const calls = [];
		const result = warmMainDependencyCaches({ functionalHit, unitHit, execute(args) {
			calls.push(args);
			if (args[0] === "build") return "";
			if (args[1] === "-m") return "owner/product\n";
			return "owner/product\nowner/product/pkg/example\nowner/product/pkg/example.test\nnet/http\nthird/party\nthird/party\nexample [test]\n";
		} });
		if (functionalHit === "true" && unitHit === "true") {
			assert.deepEqual(calls, []);
			assert.equal(result.warmed, false);
		} else {
			assert.deepEqual(calls.at(-1), ["build", "-p=4", "net/http", "runtime/coverage", "third/party"]);
			assert.equal(result.warmed, true);
			assert.ok(calls.every((args) => args[0] !== "test"));
		}
	}
	assert.throws(() => warmMainDependencyCaches({ execute() { throw new Error("dependency unavailable"); } }), /dependency unavailable/);
});
