import assert from "node:assert/strict";
import test from "node:test";

import {
	BASELINE_BOT_BRANCH,
	MAIN_CI_CHURN_COMMENT_MARKER,
	createGitHubClient,
	collectMainCiChurnReport,
	parseArguments,
	reduceMainCiChurnReport,
	renderMainCiChurnJson,
	renderMainCiChurnMarkdown,
	validateQuery,
} from "./main-ci-churn-report.mjs";

const QUERY = {
	repository: "portpowered/you-agent-factory",
	workflow: "ci.yml",
	branch: "main",
	since: "2026-01-01T00:00:00Z",
	until: "2026-01-02T00:00:00Z",
	runLimit: 100,
	mergeLimit: 200,
	format: "json",
};

function run(id, conclusion, createdAt, startedAt, actor = { login: "human", type: "User" }) {
	return {
		id,
		status: "completed",
		event: "push",
		head_branch: "main",
		conclusion,
		created_at: createdAt,
		run_started_at: startedAt,
		html_url: `https://github.com/portpowered/you-agent-factory/actions/runs/${id}`,
		actor,
	};
}

function job(id, startedAt, completedAt) {
	return {
		id,
		status: "completed",
		started_at: startedAt,
		completed_at: completedAt,
	};
}

function pullRequest(number, mergedAt, { head = "feature", title = "feature", bot = false } = {}) {
	return {
		number,
		updated_at: mergedAt,
		merged_at: mergedAt,
		base: { ref: "main" },
		head: { ref: head },
		title,
		html_url: `https://github.com/portpowered/you-agent-factory/pull/${number}`,
		user: bot ? { login: "automation[bot]", type: "Bot" } : { login: "human", type: "User" },
		merged_by: bot ? { login: "automation[bot]", type: "Bot" } : { login: "human", type: "User" },
	};
}

function response(payload, status = 200) {
	return {
		ok: status >= 200 && status < 300,
		status,
		headers: { get: () => null },
		json: async () => payload,
	};
}

function fixture() {
	const runs = [
		run(1, "success", "2026-01-01T01:00:00Z", "2026-01-01T01:00:05Z"),
		run(2, "failure", "2026-01-01T02:00:00Z", "2026-01-01T02:00:05Z", { login: "github-actions[bot]", type: "Bot" }),
		run(3, "cancelled", "2026-01-01T03:00:00Z", "2026-01-01T03:00:05Z"),
		run(4, "cancelled", "2026-01-01T04:00:00Z", null),
	];
	const jobsByRunId = new Map([
		[1, [job(11, "2026-01-01T01:00:10Z", "2026-01-01T01:00:20Z"), job(12, "2026-01-01T01:01:00Z", "2026-01-01T01:01:20Z")]],
		[2, [job(21, "2026-01-01T02:00:10Z", "2026-01-01T02:00:15Z")]],
		[3, [job(31, "2026-01-01T03:00:10Z", "2026-01-01T03:00:50Z")]],
		[4, []],
	]);
	const merges = [
		pullRequest(101, "2026-01-01T01:10:00Z"),
		pullRequest(102, "2026-01-01T02:10:00Z", { head: BASELINE_BOT_BRANCH, bot: true }),
		pullRequest(103, "2026-01-01T03:10:00Z", { title: "chore(ci): reconcile shared CI baselines", bot: true }),
	];
	return { runs, jobsByRunId, merges };
}

test("the reducer measures outcomes, burn position, attribution, and normalized ratios", () => {
	const data = fixture();
	const report = reduceMainCiChurnReport({ query: QUERY, ...data });

	assert.equal(report.schemaVersion, 1);
	assert.deepEqual(report.mainCi.outcomes, { success: 1, failure: 1, cancelled: 2 });
	assert.equal(report.mainCi.completedRuns, 4);
	assert.equal(report.mainCi.totalJobSeconds, 75);
	assert.deepEqual(report.mainCi.cancellation, {
		allRate: 0.5,
		startedRuns: 1,
		startedRate: 0.25,
		queuedRuns: 1,
		meanStartedCancelledJobSeconds: 40,
		meanSuccessfulJobSeconds: 30,
		startedBurnFractionOfSuccess: 1.333,
	});
	assert.deepEqual(report.mainCi.pushes, { total: 4, bot: 1, nonBot: 3 });
	assert.deepEqual(report.mergedChanges, {
		total: 3,
		baselineBot: 2,
		bot: 2,
		nonBot: 1,
		baselineBotShare: 0.667,
		links: [
			{ number: 101, url: "https://github.com/portpowered/you-agent-factory/pull/101", baselineBot: false },
			{ number: 102, url: "https://github.com/portpowered/you-agent-factory/pull/102", baselineBot: true },
			{ number: 103, url: "https://github.com/portpowered/you-agent-factory/pull/103", baselineBot: true },
		],
	});
	assert.deepEqual(report.normalized, {
		mainCiJobSecondsPerMergedChange: 25,
		baselineBotMergesPerNonBotMerge: 2,
	});
});

test("the reducer treats queued cancellations as zero started burn and preserves unavailable denominators", () => {
	const data = fixture();
	data.runs = [data.runs[3]];
	data.jobsByRunId = new Map([[4, []]]);
	data.merges = [];
	const report = reduceMainCiChurnReport({ query: QUERY, ...data });

	assert.equal(report.mainCi.cancellation.startedRuns, 0);
	assert.equal(report.mainCi.cancellation.queuedRuns, 1);
	assert.equal(report.mainCi.cancellation.meanStartedCancelledJobSeconds, 0);
	assert.equal(report.mainCi.cancellation.meanSuccessfulJobSeconds, null);
	assert.equal(report.mainCi.cancellation.startedBurnFractionOfSuccess, null);
	assert.equal(report.normalized.mainCiJobSecondsPerMergedChange, null);
	assert.equal(report.normalized.baselineBotMergesPerNonBotMerge, null);
});

test("the adapter follows duplicate boundary records without double counting", async () => {
	const calls = [];
	const manyRuns = Array.from({ length: 101 }, (_, index) => {
		const minute = String(index).padStart(2, "0");
		return run(index + 1, "success", `2026-01-01T01:${minute}:00Z`, `2026-01-01T01:${minute}:05Z`);
	});
	const manyJobs = Array.from({ length: 101 }, (_, index) => job(
		index + 1,
		`2026-01-01T01:00:00Z`,
		`2026-01-01T01:00:01Z`,
	));
	const client = createGitHubClient({
		repository: QUERY.repository,
		token: "test-token",
		apiBaseUrl: "https://api.example.test",
		fetchImpl: async (url) => {
			const parsed = new URL(url);
			calls.push(parsed);
			if (parsed.pathname.endsWith("/actions/workflows/ci.yml/runs")) {
				const page = Number(parsed.searchParams.get("page"));
				return response({
					total_count: 101,
					workflow_runs: page === 1 ? manyRuns.slice(0, 100) : [manyRuns[99], manyRuns[100]],
				});
			}
			if (parsed.pathname.endsWith("/actions/runs/1/jobs")) {
				const page = Number(parsed.searchParams.get("page"));
				return response({
					total_count: 101,
					jobs: page === 1 ? manyJobs.slice(0, 100) : [manyJobs[99], manyJobs[100]],
				});
			}
			if (parsed.pathname.endsWith("/pulls")) {
				return response([
					pullRequest(201, "2026-01-01T05:00:00Z"),
					pullRequest(202, "2026-01-01T04:00:00Z"),
				]);
			}
			throw new Error(`unexpected endpoint ${parsed.pathname}`);
		},
	});

	const query = validateQuery({ ...QUERY, runLimit: 101 });
	const selected = await client.listWorkflowRuns(query);
	assert.equal(selected.length, 101);
	assert.equal(new Set(selected.map((item) => item.id)).size, 101);
	const jobs = await client.listRunJobs(1, { limit: 101 });
	assert.equal(jobs.length, 101);
	assert.equal(new Set(jobs.map((item) => item.id)).size, 101);
	const pullRequests = await client.listClosedPullRequests(query);
	assert.deepEqual(pullRequests.map((item) => item.number), [201, 202]);
	assert.equal(client.requestCount, 5);
	assert.ok(calls.every((url) => url.searchParams.get("per_page") === "100"));
});

test("the adapter fails closed on a truncated page and dependency failures", async () => {
	const query = validateQuery(QUERY);
	const truncated = createGitHubClient({
		repository: query.repository,
		token: "test-token",
		fetchImpl: async () => response({ total_count: 2, workflow_runs: [run(1, "success", "2026-01-01T01:00:00Z", null)] }),
	});
	await assert.rejects(
		truncated.listWorkflowRuns({ ...query, runLimit: 2 }),
		/ended before total_count 2 was observed/,
	);

	const unauthorized = createGitHubClient({
		repository: query.repository,
		token: "test-token",
		fetchImpl: async () => response({ message: "bad token" }, 401),
	});
	await assert.rejects(unauthorized.listWorkflowRuns(query), /authorization failed/);

	const timedOut = createGitHubClient({
		repository: query.repository,
		token: "test-token",
		timeoutMs: 10,
		fetchImpl: async () => {
			const error = new Error("aborted by test");
			error.name = "AbortError";
			throw error;
		},
	});
	await assert.rejects(timedOut.listWorkflowRuns(query), /timed out/);
});

test("invalid inputs and inconsistent records fail without a success result", () => {
	assert.throws(() => validateQuery({ ...QUERY, until: QUERY.since }), /until must be later than since/);
	assert.throws(() => validateQuery({ ...QUERY, since: "2026-02-30T00:00:00Z" }), /valid calendar timestamp/);
	assert.throws(() => validateQuery({ ...QUERY, runLimit: 0 }), /runLimit must be an integer/);
	assert.throws(() => parseArguments(["--repository", QUERY.repository]), /--workflow is required/);
	assert.throws(() => parseArguments(["--unknown", "value"]), /unknown argument/);

	const data = fixture();
	const inconsistent = { ...data.runs[0], conclusion: "failure" };
	assert.throws(
		() => reduceMainCiChurnReport({ query: QUERY, ...data, runs: [data.runs[0], inconsistent] }),
		/inconsistent duplicate id 1/,
	);
	assert.throws(
		() => reduceMainCiChurnReport({
			query: QUERY,
			runs: data.runs,
			jobsByRunId: new Map([[1, [job(11, "2026-01-01T01:00:20Z", "2026-01-01T01:00:10Z")]], [2, []], [3, []], [4, []]]),
			merges: data.merges,
		}),
		/negative duration/,
	);
	assert.throws(
		() => reduceMainCiChurnReport({ query: QUERY, runs: data.runs, jobsByRunId: new Map(), merges: data.merges }),
		/missing job page for workflow run 1/,
	);
});

test("the command parser and renderers preserve a versioned, comment-ready contract", () => {
	const parsed = parseArguments([
		"--repository", QUERY.repository,
		"--workflow", QUERY.workflow,
		"--branch", QUERY.branch,
		"--since", QUERY.since,
		"--until", QUERY.until,
		"--run-limit", "100",
		"--merge-limit", "200",
		"--format", "markdown",
	]);
	assert.equal(parsed.format, "markdown");

	const report = reduceMainCiChurnReport({ query: parsed, ...fixture() });
	const json = renderMainCiChurnJson(report);
	assert.deepEqual(JSON.parse(json), report);
	const markdown = renderMainCiChurnMarkdown(report);
	assert.ok(markdown.startsWith(MAIN_CI_CHURN_COMMENT_MARKER));
	assert.match(markdown, /Main CI job-seconds per merged change: `25`/);
	assert.match(markdown, /actions\/runs\/1/);
	assert.match(markdown, /pull\/102/);
	assert.match(markdown, /writes the result to stdout/);
});

test("the collection boundary uses the injected client and keeps report generation read-only", async () => {
	const data = fixture();
	const calls = [];
	const report = await collectMainCiChurnReport(QUERY, {
		client: {
			async listWorkflowRuns() {
				calls.push("runs");
				return data.runs;
			},
			async listRunJobs(id) {
				calls.push(`jobs:${id}`);
				return data.jobsByRunId.get(id);
			},
			async listClosedPullRequests() {
				calls.push("pulls");
				return data.merges;
			},
		},
	});
	assert.deepEqual(calls, ["runs", "jobs:1", "jobs:2", "jobs:3", "jobs:4", "pulls"]);
	assert.equal(report.schemaVersion, 1);
	assert.equal(report.mainCi.completedRuns, 4);
});
