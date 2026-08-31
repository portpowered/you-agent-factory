import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

export const REPORT_SCHEMA_VERSION = 1;
export const REPORT_VERSION = REPORT_SCHEMA_VERSION;
export const MAIN_CI_CHURN_COMMENT_MARKER = "<!-- main-ci-churn-report -->";
export const BASELINE_BOT_BRANCH = "automation/shared-ci-baselines";
export const BASELINE_BOT_PR_TITLE = "chore(ci): reconcile shared CI baselines";
export const DEFAULT_GITHUB_API_URL = "https://api.github.com";
export const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
export const DEFAULT_PAGE_SIZE = 100;
export const MAX_QUERY_LIMIT = 1000;
export const MAX_API_REQUESTS = 120;

const RFC3339_PATTERN =
	/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;
const HTTPS_URL_PATTERN = /^https:\/\/[^\s]+$/;
const QUERY_FIELDS = [
	"repository",
	"workflow",
	"branch",
	"since",
	"until",
	"runLimit",
	"mergeLimit",
];

export class MainCiChurnReportError extends Error {
	constructor(message, options = {}) {
		super(message, options);
	this.name = "MainCiChurnReportError";
	}
}

function fail(message) {
	throw new MainCiChurnReportError(message);
}

function isRecord(value) {
	return value !== null && typeof value === "object" && !Array.isArray(value);
}

function requiredText(value, label) {
	if (typeof value !== "string" || value.trim() === "") {
		fail(`${label} must be a non-empty string`);
	}
	return value.trim();
}

function positiveInteger(value, label, maximum = Number.MAX_SAFE_INTEGER) {
	const number = typeof value === "number" ? value : Number(String(value).trim());
	if (!Number.isSafeInteger(number) || number < 1 || number > maximum) {
		fail(`${label} must be an integer from 1 through ${maximum}`);
	}
	return number;
}

function parseRfc3339(value, label) {
	if (typeof value !== "string") {
		fail(`${label} must be an RFC3339 timestamp`);
	}
	const match = value.match(RFC3339_PATTERN);
	if (!match) {
		fail(`${label} must be an RFC3339 timestamp`);
	}
	const [, yearText, monthText, dayText, hourText, minuteText, secondText, , offset] = match;
	const year = Number(yearText);
	const month = Number(monthText);
	const day = Number(dayText);
	const hour = Number(hourText);
	const minute = Number(minuteText);
	const second = Number(secondText);
	if (month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59) {
		fail(`${label} is not a valid calendar timestamp`);
	}
	const calendar = new Date(Date.UTC(year, month - 1, day));
	if (
		calendar.getUTCFullYear() !== year
		|| calendar.getUTCMonth() !== month - 1
		|| calendar.getUTCDate() !== day
	) {
		fail(`${label} is not a valid calendar timestamp`);
	}
	if (offset !== "Z") {
		const offsetHours = Number(offset.slice(1, 3));
		const offsetMinutes = Number(offset.slice(4, 6));
		if (offsetHours > 23 || offsetMinutes > 59) {
			fail(`${label} has an invalid timezone offset`);
		}
	}
	const milliseconds = Date.parse(value);
	if (!Number.isFinite(milliseconds)) {
		fail(`${label} must be an RFC3339 timestamp`);
	}
	return milliseconds;
}

function canonicalTimestamp(value, label) {
	return new Date(parseRfc3339(value, label)).toISOString();
}

function timestampOrNull(value, label) {
	if (value === null || value === undefined) return null;
	return parseRfc3339(value, label);
}

function requiredHttpsUrl(value, label) {
	const url = requiredText(value, label);
	if (!HTTPS_URL_PATTERN.test(url)) {
		fail(`${label} must be an https URL`);
	}
	try {
		new URL(url);
	} catch {
		fail(`${label} must be an https URL`);
	}
	return url;
}

function stableStringify(value) {
	if (Array.isArray(value)) {
		return `[${value.map(stableStringify).join(",")}]`;
	}
	if (isRecord(value)) {
		return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stableStringify(value[key])}`).join(",")}}`;
	}
	return JSON.stringify(value);
}

function deduplicateConsistent(records, label, idOf) {
	if (!Array.isArray(records)) {
		fail(`${label} must be an array`);
	}
	const seen = new Map();
	for (const record of records) {
		const id = idOf(record);
		const prior = seen.get(id);
		if (prior && stableStringify(prior) !== stableStringify(record)) {
			fail(`${label} contained inconsistent duplicate id ${id}`);
		}
		if (!prior) seen.set(id, record);
	}
	return [...seen.values()];
}

function recordId(value, label) {
	if (!isRecord(value)) fail(`${label} contains a malformed record`);
	return positiveInteger(value.id, `${label} id`);
}

function actorIdentity(value, label) {
	if (value === null || value === undefined) return null;
	if (!isRecord(value)) fail(`${label} must be an object or null`);
	if (value.login !== undefined && value.login !== null && typeof value.login !== "string") {
		fail(`${label}.login must be a string`);
	}
	if (value.type !== undefined && value.type !== null && typeof value.type !== "string") {
		fail(`${label}.type must be a string`);
	}
	return {
		login: value.login ? value.login.trim() : "",
		type: value.type ? value.type.trim() : "",
	};
}

export function isBotIdentity(value) {
	const identity = value || {};
	return (identity.type || "").toLowerCase() === "bot" || /\[bot\]$/i.test(identity.login || "");
}

export function validateQuery(input = {}) {
	if (!isRecord(input)) fail("query must be an object");
	const repository = requiredText(input.repository, "repository");
	if (!/^[^/\s]+\/[^/\s]+$/.test(repository)) {
		fail("repository must have the owner/name form");
	}
	const workflow = requiredText(input.workflow, "workflow");
	if (/\s/.test(workflow)) fail("workflow must not contain whitespace");
	const branch = requiredText(input.branch, "branch");
	const since = canonicalTimestamp(input.since, "since");
	const until = canonicalTimestamp(input.until, "until");
	if (Date.parse(until) <= Date.parse(since)) {
		fail("until must be later than since");
	}
	const runLimit = positiveInteger(input.runLimit, "runLimit", MAX_QUERY_LIMIT);
	const mergeLimit = positiveInteger(input.mergeLimit, "mergeLimit", MAX_QUERY_LIMIT);
	const format = input.format === undefined ? "json" : requiredText(input.format, "format").toLowerCase();
	if (format !== "json" && format !== "markdown") {
		fail("format must be json or markdown");
	}
	return {
		repository,
		workflow,
		branch,
		since,
		until,
		runLimit,
		mergeLimit,
		format,
	};
}

function normalizeApiRun(run, query, { strictIdentity = false } = {}) {
	if (!isRecord(run)) fail("workflow runs contained a malformed record");
	const id = recordId(run, "workflow run");
	if (strictIdentity || run.status !== undefined) {
		if (run.status !== "completed") fail(`workflow run ${id} is not completed`);
	}
	if (strictIdentity || run.event !== undefined) {
		if (run.event !== "push") fail(`workflow run ${id} is not a push event`);
	}
	if (strictIdentity || run.head_branch !== undefined) {
		if (run.head_branch !== query.branch) {
			fail(`workflow run ${id} is not on branch ${query.branch}`);
		}
	}
	const conclusion = requiredText(run.conclusion, `workflow run ${id} conclusion`).toLowerCase();
	const createdAt = parseRfc3339(run.created_at, `workflow run ${id} created_at`);
	const since = Date.parse(query.since);
	const until = Date.parse(query.until);
	if (createdAt < since || createdAt > until) {
		fail(`workflow run ${id} falls outside the stated window`);
	}
	if (run.updated_at !== undefined && run.updated_at !== null) {
		parseRfc3339(run.updated_at, `workflow run ${id} updated_at`);
	}
	const startedValue = Object.hasOwn(run, "run_started_at") ? run.run_started_at : run.started_at;
	const startedAt = timestampOrNull(startedValue, `workflow run ${id} run_started_at`);
	const url = requiredHttpsUrl(run.html_url || run.url, `workflow run ${id} html_url`);
	const actor = actorIdentity(run.actor, `workflow run ${id} actor`);
	const triggeringActor = actorIdentity(run.triggering_actor, `workflow run ${id} triggering_actor`);
	return {
		id,
		conclusion,
		createdAt,
		startedAt,
		url,
		bot: isBotIdentity(actor) || isBotIdentity(triggeringActor),
	};
}

function durationSeconds(job, label) {
	if (!isRecord(job)) fail(`${label} is malformed`);
	if (job.conclusion === "skipped") return 0;
	if (Object.hasOwn(job, "durationSeconds")) {
		const duration = Number(job.durationSeconds);
		if (!Number.isFinite(duration) || duration < 0) fail(`${label} has a negative or invalid duration`);
		return duration;
	}
	const startedAt = timestampOrNull(job.started_at, `${label} started_at`);
	const completedAt = timestampOrNull(job.completed_at, `${label} completed_at`);
	if (startedAt === null && completedAt === null) return 0;
	if (startedAt === null || completedAt === null) {
		fail(`${label} must have both started_at and completed_at when either is present`);
	}
	const duration = (completedAt - startedAt) / 1000;
	if (duration < 0) fail(`${label} has a negative duration`);
	return duration;
}

function normalizeJobs(jobs, runId, { strictIdentity = false } = {}) {
	if (!Array.isArray(jobs)) fail(`jobs for workflow run ${runId} must be an array`);
	const uniqueJobs = deduplicateConsistent(jobs, `jobs for workflow run ${runId}`, (job) => recordId(job, "job"));
	let seconds = 0;
	for (const job of uniqueJobs) {
		const id = recordId(job, `job for workflow run ${runId}`);
		if (strictIdentity || job.status !== undefined) {
			if (job.status !== "completed") fail(`job ${id} for workflow run ${runId} is not completed`);
		}
		seconds += durationSeconds(job, `job ${id} for workflow run ${runId}`);
	}
	return seconds;
}

function jobsForRun(jobsByRunId, runId) {
	if (jobsByRunId instanceof Map) {
		return jobsByRunId.has(runId) ? jobsByRunId.get(runId) : undefined;
	}
	if (isRecord(jobsByRunId)) {
		return Object.hasOwn(jobsByRunId, String(runId)) ? jobsByRunId[String(runId)] : undefined;
	}
	if (Array.isArray(jobsByRunId)) {
		const entry = jobsByRunId.find((candidate) => candidate?.runId === runId);
		return entry?.jobs;
	}
	return undefined;
}

function normalizeMerge(pullRequest, query, { strictIdentity = false } = {}) {
	if (!isRecord(pullRequest)) fail("pull requests contained a malformed record");
	const number = positiveInteger(pullRequest.number, "pull request number");
	const mergedValue = pullRequest.merged_at ?? pullRequest.pull_request?.merged_at;
	const mergedAt = timestampOrNull(mergedValue, `pull request ${number} merged_at`);
	const updatedAt = parseRfc3339(pullRequest.updated_at, `pull request ${number} updated_at`);
	const baseRef = pullRequest.base?.ref;
	if (baseRef !== undefined) {
		if (baseRef !== query.branch) fail(`pull request ${number} is not based on branch ${query.branch}`);
	}
	const url = requiredHttpsUrl(pullRequest.html_url || pullRequest.url, `pull request ${number} html_url`);
	if (mergedAt === null || mergedAt < Date.parse(query.since) || mergedAt > Date.parse(query.until)) {
		return { number, mergedAt, updatedAt, url, relevant: false };
	}
	const headRef = pullRequest.head?.ref;
	const author = actorIdentity(pullRequest.user, `pull request ${number} user`);
	const merger = actorIdentity(pullRequest.merged_by, `pull request ${number} merged_by`);
	const title = typeof pullRequest.title === "string" ? pullRequest.title : "";
	const baselineBot = headRef === BASELINE_BOT_BRANCH || title === BASELINE_BOT_PR_TITLE;
	const bot = baselineBot || isBotIdentity(author) || isBotIdentity(merger);
	return {
		number,
		mergedAt,
		updatedAt,
		url,
		baselineBot,
		bot,
		relevant: true,
	};
}

function formatMetric(value) {
	if (value === null || value === undefined) return null;
	if (!Number.isFinite(value)) return null;
	return Math.round(value * 1000) / 1000;
}

function outputQuery(query) {
	return {
		repository: query.repository,
		workflow: query.workflow,
		branch: query.branch,
		since: query.since,
		until: query.until,
		runLimit: query.runLimit,
		mergeLimit: query.mergeLimit,
	};
}

function outputJobsMap(jobsByRunId, runIds) {
	const values = new Map();
	for (const runId of runIds) {
		const jobs = jobsForRun(jobsByRunId, runId);
		if (jobs === undefined) fail(`missing job page for workflow run ${runId}`);
		values.set(runId, jobs);
	}
	return values;
}

export function reduceMainCiChurnReport({ query: rawQuery, runs, jobsByRunId, merges } = {}, options = {}) {
	const query = validateQuery(rawQuery);
	const strictApiRecords = options.strictApiRecords === true;
	if (!Array.isArray(runs)) fail("workflow runs must be an array");
	if (!Array.isArray(merges)) fail("pull requests must be an array");
	const rawRuns = deduplicateConsistent(runs, "workflow runs", (run) => recordId(run, "workflow run"));
	if (rawRuns.length === 0) fail("no completed main CI workflow runs were returned");
	const normalizedRuns = rawRuns
		.map((run) => normalizeApiRun(run, query, { strictIdentity: strictApiRecords }))
		.sort((left, right) => left.createdAt - right.createdAt || left.id - right.id);
	const jobMap = outputJobsMap(jobsByRunId, normalizedRuns.map((run) => run.id));
	const measurements = normalizedRuns.map((run) => ({
		run,
		jobSeconds: normalizeJobs(jobMap.get(run.id), run.id, { strictIdentity: strictApiRecords }),
	}));
	const rawMerges = deduplicateConsistent(merges, "pull requests", (pullRequest) => {
		if (!isRecord(pullRequest)) fail("pull requests contained a malformed record");
		return positiveInteger(pullRequest.number, "pull request number");
	});
	const normalizedMerges = rawMerges
		.map((pullRequest) => normalizeMerge(pullRequest, query, { strictIdentity: strictApiRecords }))
		.filter((pullRequest) => pullRequest.relevant)
		.sort((left, right) => left.mergedAt - right.mergedAt || left.number - right.number);

	const outcomes = {};
	let totalJobSeconds = 0;
	let cancelledRuns = 0;
	let startedCancelledRuns = 0;
	let queuedCancelledRuns = 0;
	let successfulSeconds = 0;
	let successfulRuns = 0;
	let startedCancelledSeconds = 0;
	for (const { run, jobSeconds } of measurements) {
		outcomes[run.conclusion] = (outcomes[run.conclusion] || 0) + 1;
		totalJobSeconds += jobSeconds;
		if (run.conclusion === "success") {
			successfulRuns += 1;
			successfulSeconds += jobSeconds;
		}
		if (run.conclusion === "cancelled") {
			cancelledRuns += 1;
			if (run.startedAt === null) queuedCancelledRuns += 1;
			else {
				startedCancelledRuns += 1;
				startedCancelledSeconds += jobSeconds;
			}
		}
	}
	const totalRuns = measurements.length;
	const successfulMean = successfulRuns > 0 ? successfulSeconds / successfulRuns : null;
	const startedCancelledMean = startedCancelledRuns > 0
		? startedCancelledSeconds / startedCancelledRuns
		: 0;
	const botPushes = measurements.filter(({ run }) => run.bot).length;
	const totalMerges = normalizedMerges.length;
	const baselineBotMerges = normalizedMerges.filter((pullRequest) => pullRequest.baselineBot).length;
	const botMerges = normalizedMerges.filter((pullRequest) => pullRequest.bot).length;
	const nonBotMerges = totalMerges - botMerges;
	const report = {
		schemaVersion: REPORT_SCHEMA_VERSION,
		query: outputQuery(query),
		mainCi: {
			completedRuns: totalRuns,
			outcomes,
			totalJobSeconds: formatMetric(totalJobSeconds),
			cancellation: {
				allRate: formatMetric(cancelledRuns / totalRuns),
				startedRuns: startedCancelledRuns,
				startedRate: formatMetric(startedCancelledRuns / totalRuns),
				queuedRuns: queuedCancelledRuns,
				meanStartedCancelledJobSeconds: formatMetric(startedCancelledMean),
				meanSuccessfulJobSeconds: formatMetric(successfulMean),
				startedBurnFractionOfSuccess: formatMetric(
					successfulMean === null || successfulMean === 0 ? null : startedCancelledMean / successfulMean,
				),
			},
			pushes: {
				total: totalRuns,
				bot: botPushes,
				nonBot: totalRuns - botPushes,
			},
			runLinks: measurements.map(({ run }) => ({ id: run.id, url: run.url, conclusion: run.conclusion })),
		},
		mergedChanges: {
			total: totalMerges,
			baselineBot: baselineBotMerges,
			bot: botMerges,
			nonBot: nonBotMerges,
			baselineBotShare: totalMerges > 0 ? formatMetric(baselineBotMerges / totalMerges) : null,
			links: normalizedMerges.map((pullRequest) => ({
				number: pullRequest.number,
				url: pullRequest.url,
				baselineBot: pullRequest.baselineBot,
			})),
		},
		normalized: {
			mainCiJobSecondsPerMergedChange: totalMerges > 0
				? formatMetric(totalJobSeconds / totalMerges)
				: null,
			baselineBotMergesPerNonBotMerge: nonBotMerges > 0
				? formatMetric(baselineBotMerges / nonBotMerges)
				: null,
		},
	};
	return report;
}

function pageItems(payload, key, label) {
	if (!isRecord(payload) || !Array.isArray(payload[key])) {
		fail(`${label} returned a malformed page`);
	}
	return payload[key];
}

function pageTotal(payload, label) {
	if (!isRecord(payload) || !Object.hasOwn(payload, "total_count")) {
		fail(`${label} page omitted total_count`);
	}
	const total = Number(payload.total_count);
	if (!Number.isSafeInteger(total) || total < 0) {
		fail(`${label} page returned an invalid total_count`);
	}
	return total;
}

async function fetchPages({
	label,
	limit,
	pageSize,
	fetchPage,
	itemsOf,
	totalOf = null,
	terminal = null,
	idOf = (record) => recordId(record, label),
}) {
	const maxPages = Math.ceil(limit / pageSize) + 1;
	let expectedTotal = null;
	let previousPageSignature = null;
	const records = [];
	for (let page = 1; page <= maxPages; page += 1) {
		const payload = await fetchPage(page, pageSize);
		const items = itemsOf(payload, label);
		if (items.length > pageSize) fail(`${label} page ${page} exceeded its requested page size`);
		if (totalOf) {
			const pageTotalValue = totalOf(payload, label);
			if (expectedTotal === null) expectedTotal = pageTotalValue;
			else if (pageTotalValue !== expectedTotal) fail(`${label} pages reported inconsistent total_count`);
			if (expectedTotal > limit) {
				fail(`${label} pagination is truncated by limit ${limit}; increase the limit for a complete report`);
			}
		}
		const pageSignature = stableStringify(items);
		if (pageSignature === previousPageSignature && items.length > 0) {
			fail(`${label} returned the same page twice`);
		}
		previousPageSignature = pageSignature;
		records.push(...items);
		const uniqueCount = deduplicateConsistent(records, label, idOf).length;
		if (uniqueCount > limit) fail(`${label} pagination exceeded limit ${limit}`);
		const isTerminal = terminal
			? terminal(items, page, pageSize)
			: items.length < pageSize;
		const countComplete = expectedTotal !== null && uniqueCount === expectedTotal;
		if (isTerminal || countComplete) {
			if (expectedTotal !== null && uniqueCount !== expectedTotal) {
				fail(`${label} pagination ended before total_count ${expectedTotal} was observed`);
			}
			return deduplicateConsistent(records, label, idOf);
		}
		if (page === maxPages) {
			fail(`${label} pagination exceeded its bounded page budget; refusing an incomplete report`);
		}
	}
	fail(`${label} pagination did not produce a complete result`);
}

function repositoryPath(repository) {
	return repository.split("/").map(encodeURIComponent).join("/");
}

function addQueryParameters(url, parameters) {
	for (const [key, value] of Object.entries(parameters)) {
		if (value !== undefined && value !== null) url.searchParams.set(key, String(value));
	}
}

export function createGitHubClient({
	repository,
	token = process.env.GH_TOKEN,
	fetchImpl = globalThis.fetch,
	apiBaseUrl = DEFAULT_GITHUB_API_URL,
	timeoutMs = DEFAULT_REQUEST_TIMEOUT_MS,
	maxRequests = MAX_API_REQUESTS,
} = {}) {
	const validatedRepository = requiredText(repository, "repository");
	if (!/^[^/\s]+\/[^/\s]+$/.test(validatedRepository)) fail("repository must have the owner/name form");
	if (!requiredText(token, "GH_TOKEN")) fail("GH_TOKEN is required for GitHub API access");
	if (typeof fetchImpl !== "function") fail("a fetch implementation is required");
	if (!Number.isSafeInteger(timeoutMs) || timeoutMs < 1) fail("timeoutMs must be a positive integer");
	if (!Number.isSafeInteger(maxRequests) || maxRequests < 1) fail("maxRequests must be a positive integer");
	const base = new URL(apiBaseUrl.endsWith("/") ? apiBaseUrl : `${apiBaseUrl}/`);
	let requestCount = 0;

	async function request(path, parameters, label) {
		requestCount += 1;
		if (requestCount > maxRequests) {
			fail(`GitHub API request budget exceeded ${maxRequests}; refusing an unbounded report`);
		}
		const endpoint = path.startsWith("search/")
			? path
			: `repos/${repositoryPath(validatedRepository)}/${path}`;
		const url = new URL(endpoint, base);
		addQueryParameters(url, parameters);
		const controller = new AbortController();
		const timeout = setTimeout(() => controller.abort(), timeoutMs);
		let response;
		try {
			response = await fetchImpl(url.toString(), {
				method: "GET",
				headers: {
					Accept: "application/vnd.github+json",
					Authorization: `Bearer ${token}`,
					"X-GitHub-Api-Version": "2022-11-28",
					"User-Agent": "you-agent-factory-main-ci-churn-report",
				},
				signal: controller.signal,
			});
		} catch (error) {
			if (error?.name === "AbortError") fail(`GitHub API request timed out while reading ${label}`);
			fail(`GitHub API request failed while reading ${label}: ${error?.message || "unknown network error"}`);
		} finally {
			clearTimeout(timeout);
		}
		const status = Number(response?.status);
		if (!Number.isInteger(status) || status < 200 || status >= 300) {
			if (status === 401 || status === 403) fail(`GitHub API authorization failed while reading ${label} (HTTP ${status})`);
			fail(`GitHub API request failed while reading ${label} (HTTP ${status || "unknown"})`);
		}
		try {
			return await response.json();
		} catch (error) {
			fail(`GitHub API returned malformed JSON while reading ${label}: ${error?.message || "invalid JSON"}`);
		}
	}

	return {
		get requestCount() {
			return requestCount;
		},
		listWorkflowRuns(query) {
			const pageSize = Math.min(DEFAULT_PAGE_SIZE, query.runLimit);
			return fetchPages({
				label: "workflow runs",
				limit: query.runLimit,
				pageSize,
				fetchPage: (page, perPage) => request(
					`actions/workflows/${encodeURIComponent(query.workflow)}/runs`,
					{
						branch: query.branch,
						event: "push",
						status: "completed",
						created: `${query.since}..${query.until}`,
						per_page: perPage,
						page,
					},
					"workflow runs",
				),
				itemsOf: (payload, label) => pageItems(payload, "workflow_runs", label),
				totalOf: pageTotal,
			});
		},
		listRunJobs(runId, options = {}) {
			const limit = options.limit || MAX_QUERY_LIMIT;
			const pageSize = Math.min(DEFAULT_PAGE_SIZE, limit);
			return fetchPages({
				label: `jobs for workflow run ${runId}`,
				limit,
				pageSize,
				fetchPage: (page, perPage) => request(
					`actions/runs/${positiveInteger(runId, "workflow run id")}/jobs`,
					{per_page: perPage, page},
					`jobs for workflow run ${runId}`,
				),
				itemsOf: (payload, label) => pageItems(payload, "jobs", label),
				totalOf: pageTotal,
			});
		},
		listClosedPullRequests(query) {
			const pageSize = Math.min(DEFAULT_PAGE_SIZE, query.mergeLimit);
			return fetchPages({
				label: "closed pull requests",
				limit: query.mergeLimit,
				pageSize,
				fetchPage: (page, perPage) => request(
					"pulls",
					{
						state: "closed",
						base: query.branch,
						sort: "updated",
						direction: "desc",
						per_page: perPage,
						page,
					},
					"closed pull requests",
				),
				itemsOf: (payload, label) => {
					if (!Array.isArray(payload)) fail(`${label} returned a malformed page`);
					for (const pullRequest of payload) {
						if (!isRecord(pullRequest)) fail(`${label} contains a malformed record`);
						parseRfc3339(pullRequest.updated_at, `pull request ${pullRequest.number || "unknown"} updated_at`);
					}
					return payload;
				},
				idOf: (pullRequest) => {
					if (!isRecord(pullRequest)) fail("closed pull requests contains a malformed record");
					return positiveInteger(pullRequest.number, "pull request number");
				},
			});
		},
		listMergedPullRequests(query) {
			const pageSize = Math.min(DEFAULT_PAGE_SIZE, query.mergeLimit);
			const sinceDate = query.since.slice(0, 10);
			const untilDate = query.until.slice(0, 10);
			return fetchPages({
				label: "merged pull requests",
				limit: query.mergeLimit,
				pageSize,
				fetchPage: (page, perPage) => request(
					"search/issues",
					{
						q: `repo:${validatedRepository} is:pr is:merged base:${query.branch} merged:${sinceDate}..${untilDate}`,
						per_page: perPage,
						page,
					},
					"merged pull requests",
				),
				itemsOf: (payload, label) => pageItems(payload, "items", label),
				totalOf: pageTotal,
				idOf: (pullRequest) => {
					if (!isRecord(pullRequest)) fail("merged pull requests contains a malformed record");
					return positiveInteger(pullRequest.number, "pull request number");
				},
			});
		},
	};
}

export async function collectMainCiChurnReport(rawQuery, { client, ...clientOptions } = {}) {
	const query = validateQuery(rawQuery);
	const github = client || createGitHubClient({ repository: query.repository, ...clientOptions });
	const runs = await github.listWorkflowRuns(query);
	if (!Array.isArray(runs)) fail("workflow runs adapter returned a non-array result");
	const uniqueRuns = deduplicateConsistent(runs, "workflow runs", (run) => recordId(run, "workflow run"));
	const jobsByRunId = new Map();
	for (const run of uniqueRuns) {
		const runId = recordId(run, "workflow run");
		const jobs = await github.listRunJobs(runId);
		if (!Array.isArray(jobs)) fail(`jobs adapter returned a non-array result for workflow run ${runId}`);
		jobsByRunId.set(runId, jobs);
	}
	const listMerges = github.listMergedPullRequests || github.listClosedPullRequests;
	if (typeof listMerges !== "function") fail("pull request adapter does not expose a merge listing method");
	const merges = await listMerges.call(github, query);
	if (!Array.isArray(merges)) fail("pull request adapter returned a non-array result");
	return reduceMainCiChurnReport(
		{ query, runs: uniqueRuns, jobsByRunId, merges },
		{ strictApiRecords: true },
	);
}

export function reportCommand(query, format = "markdown") {
	const values = validateQuery({ ...query, format });
	return [
		"node scripts/ci/main-ci-churn-report.mjs",
		"--repository", values.repository,
		"--workflow", values.workflow,
		"--branch", values.branch,
		"--since", values.since,
		"--until", values.until,
		"--run-limit", String(values.runLimit),
		"--merge-limit", String(values.mergeLimit),
		"--format", values.format,
	].join(" ");
}

function inlineCode(value) {
	return `\`${String(value).replaceAll("`", "\\`")}\``;
}

function markdownNumber(value) {
	return value === null || value === undefined ? "n/a" : String(value);
}

export function renderMainCiChurnMarkdown(report) {
	if (!isRecord(report) || report.schemaVersion !== REPORT_SCHEMA_VERSION) {
		fail(`report must have schema version ${REPORT_SCHEMA_VERSION}`);
	}
	const query = report.query;
	const mainCi = report.mainCi;
	const cancellation = mainCi.cancellation;
	const pushes = mainCi.pushes;
	const merges = report.mergedChanges;
	const normalized = report.normalized;
	const lines = [
		MAIN_CI_CHURN_COMMENT_MARKER,
		"## Main CI push-churn report",
		"",
		`- Schema version: ${inlineCode(report.schemaVersion)}`,
		`- Command: ${inlineCode(reportCommand(query, "markdown"))}`,
		`- Repository: ${inlineCode(query.repository)}`,
		`- Workflow: ${inlineCode(query.workflow)}`,
		`- Branch: ${inlineCode(query.branch)}`,
		`- Window: ${inlineCode(`${query.since} through ${query.until}`)}`,
		`- Run limit: ${inlineCode(query.runLimit)}`,
		`- Merge limit: ${inlineCode(query.mergeLimit)}`,
		"",
		"### Main CI",
		"",
		`- Completed runs: ${inlineCode(mainCi.completedRuns)}`,
		`- Total job-seconds: ${inlineCode(markdownNumber(mainCi.totalJobSeconds))}`,
		"",
		"| Outcome | Runs |",
		"| --- | ---: |",
		...Object.entries(mainCi.outcomes).map(([outcome, count]) => `| ${outcome} | ${count} |`),
		"",
		"### Cancellation",
		"",
		`- Overall cancellation rate: ${inlineCode(markdownNumber(cancellation.allRate))}`,
		`- Started cancellations: ${inlineCode(cancellation.startedRuns)}`,
		`- Started cancellation rate: ${inlineCode(markdownNumber(cancellation.startedRate))}`,
		`- Queued cancellations: ${inlineCode(cancellation.queuedRuns)}`,
		`- Mean started-cancelled job-seconds: ${inlineCode(markdownNumber(cancellation.meanStartedCancelledJobSeconds))}`,
		`- Mean successful job-seconds: ${inlineCode(markdownNumber(cancellation.meanSuccessfulJobSeconds))}`,
		`- Started cancellation burn fraction of success: ${inlineCode(markdownNumber(cancellation.startedBurnFractionOfSuccess))}`,
		"",
		"### Main push volume",
		"",
		"| Pushes | Total | Bot | Non-bot |",
		"| --- | ---: | ---: | ---: |",
		`| Main CI push-triggered runs | ${pushes.total} | ${pushes.bot} | ${pushes.nonBot} |`,
		"",
		"### Merged change volume",
		"",
		"| Merges | Total | Baseline bot | Bot | Non-bot |",
		"| --- | ---: | ---: | ---: | ---: |",
		`| Pull requests merged into ${query.branch} | ${merges.total} | ${merges.baselineBot} | ${merges.bot} | ${merges.nonBot} |`,
		`- Baseline-bot merge share: ${inlineCode(markdownNumber(merges.baselineBotShare))}`,
		"",
		"### Normalized measures",
		"",
		`- Main CI job-seconds per merged change: ${inlineCode(markdownNumber(normalized.mainCiJobSecondsPerMergedChange))}`,
		`- Baseline-bot merges per non-bot merge: ${inlineCode(markdownNumber(normalized.baselineBotMergesPerNonBotMerge))}`,
		"",
		"### Main CI run links",
		"",
		...(mainCi.runLinks.length > 0
			? mainCi.runLinks.map((run) => `- [Run ${run.id}](${run.url}) — ${run.conclusion}`)
			: ["- None."]),
		"",
		"### Merged pull-request links",
		"",
		...(merges.links.length > 0
			? merges.links.map((pullRequest) => `- [PR #${pullRequest.number}](${pullRequest.url})${pullRequest.baselineBot ? " — baseline bot" : ""}`)
			: ["- None."]),
		"",
		"This report is read-only. The command writes the result to stdout; it does not write a repository file, mutate GitHub, or commit run evidence.",
		"",
	];
	return lines.join("\n");
}

export const renderMainCiChurnReport = renderMainCiChurnMarkdown;

export function renderMainCiChurnJson(report) {
	if (!isRecord(report) || report.schemaVersion !== REPORT_SCHEMA_VERSION) {
		fail(`report must have schema version ${REPORT_SCHEMA_VERSION}`);
	}
	return `${JSON.stringify(report, null, 2)}\n`;
}

export const renderJsonReport = renderMainCiChurnJson;

function optionValue(args, index, option) {
	if (index + 1 >= args.length || args[index + 1].startsWith("--")) {
		fail(`${option} requires a value`);
	}
	return args[index + 1];
}

export function parseArguments(args = []) {
	if (!Array.isArray(args)) fail("arguments must be an array");
	const values = {};
	const names = new Map([
		["--repository", "repository"],
		["--workflow", "workflow"],
		["--branch", "branch"],
		["--since", "since"],
		["--until", "until"],
		["--run-limit", "runLimit"],
		["--merge-limit", "mergeLimit"],
		["--format", "format"],
	]);
	for (let index = 0; index < args.length; index += 2) {
		const argument = args[index];
		const field = names.get(argument);
		if (!field) fail(`unknown argument ${argument}`);
		if (Object.hasOwn(values, field)) fail(`${argument} was provided more than once`);
		values[field] = optionValue(args, index, argument);
	}
	for (const field of QUERY_FIELDS) {
		if (!Object.hasOwn(values, field)) fail(`--${field.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)} is required`);
	}
	if (!Object.hasOwn(values, "format")) fail("--format is required");
	return validateQuery(values);
}

export async function runCli(args = process.argv.slice(2), environment = process.env, output = process.stdout) {
	const query = parseArguments(args);
	const report = await collectMainCiChurnReport(query, {
		token: environment.GH_TOKEN,
		apiBaseUrl: environment.GITHUB_API_URL || DEFAULT_GITHUB_API_URL,
	});
	output.write(query.format === "json" ? renderMainCiChurnJson(report) : renderMainCiChurnMarkdown(report));
	return report;
}

if (process.argv[1] && resolve(process.argv[1]) === resolve(fileURLToPath(import.meta.url))) {
	runCli().catch((error) => {
		process.stderr.write(`main-ci-churn-report: ${error.message}\n`);
		process.exitCode = 1;
	});
}
