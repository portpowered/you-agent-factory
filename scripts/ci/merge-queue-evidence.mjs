import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { resolve, join } from "node:path";
import { fileURLToPath } from "node:url";

export const REQUIRED_ARTIFACTS = Object.freeze([
	"unit-coverage-diagnostics",
	"functional-test-diagnostics",
	"backend-deadcode-evidence",
]);
const shaPattern = /^[a-f0-9]{40}$/i;

function ghAPI(path) {
	return JSON.parse(execFileSync("gh", ["api", path], { encoding: "utf8", maxBuffer: 8 * 1024 * 1024 }));
}

// A squash commit has a different SHA from its queue commit. Only identical
// Git trees from successful CI merge groups may supply delivered evidence.
export async function resolveQueueEvidence({ repository, mainSha, api = ghAPI }) {
	if (!/^[\w.-]+\/[\w.-]+$/.test(repository) || !shaPattern.test(mainSha)) {
		throw new Error("Expected repository owner/name and a complete main commit SHA.");
	}
	const prefix = `repos/${repository}`;
	const mainTree = (await api(`${prefix}/git/commits/${mainSha}`)).tree?.sha;
	if (!shaPattern.test(mainTree ?? "")) throw new Error("Main commit tree is missing.");
	let inspected = 0;
	for (let page = 1; page <= 10; page++) {
		const response = await api(`${prefix}/actions/workflows/ci.yml/runs?event=merge_group&status=success&per_page=100&page=${page}`);
		if (!Array.isArray(response.workflow_runs)) throw new Error("Invalid queue run response.");
		for (const run of response.workflow_runs) {
			if (run.event !== "merge_group" || run.status !== "completed" || run.conclusion !== "success" ||
				run.head_repository?.full_name !== repository || !run.head_branch?.startsWith("gh-readonly-queue/main/") ||
				!shaPattern.test(run.head_sha ?? "") || !Number.isSafeInteger(run.id)) continue;
			// Stay below the task's 120-call inspection budget, even on busy repos.
			if (++inspected > 100) throw new Error("Queue evidence search exceeded its bounded commit inspection budget.");
			const tree = (await api(`${prefix}/git/commits/${run.head_sha}`)).tree?.sha;
			if (tree !== mainTree) continue;
			const artifacts = await api(`${prefix}/actions/runs/${run.id}/artifacts?per_page=100`);
			if (!Array.isArray(artifacts.artifacts) || artifacts.total_count > 100) throw new Error("Invalid or truncated artifact response.");
			for (const name of REQUIRED_ARTIFACTS) {
				const matches = artifacts.artifacts.filter((artifact) => artifact.name === name && !artifact.expired);
				if (matches.length !== 1) throw new Error(`Queue run ${run.id} requires one unexpired ${name} artifact.`);
			}
			return { repository, mainSha, tree: mainTree, queueSha: run.head_sha, runId: run.id, runAttempt: run.run_attempt, runURL: run.html_url };
		}
		if (response.workflow_runs.length < 100) break;
	}
	throw new Error("No successful main merge-group CI run has the delivered commit's exact tree; refusing substitute evidence.");
}

export async function forwardQueueEvidence(options, download = (source, name, directory) => {
	execFileSync("gh", ["run", "download", String(source.runId), "--repo", source.repository, "--name", name, "--dir", directory], { stdio: "inherit" });
}) {
	const source = await resolveQueueEvidence(options);
	const directory = resolve(options.outputDirectory);
	for (const name of REQUIRED_ARTIFACTS) {
		const destination = join(directory, name);
		mkdirSync(destination, { recursive: true });
		await download(source, name, destination);
		writeFileSync(join(destination, "queue-evidence-provenance.json"), `${JSON.stringify(source, null, 2)}\n`);
	}
	return source;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
	const args = process.argv.slice(2);
	const argument = (name) => args[args.indexOf(name) + 1];
	try {
		const source = await forwardQueueEvidence({ repository: argument("--repository"), mainSha: argument("--main-sha"), outputDirectory: argument("--output-directory") });
		console.log(`Forwarded exact-tree queue evidence: ${source.runURL}, tree ${source.tree}`);
	} catch (error) {
		console.error(error.message);
		process.exitCode = 1;
	}
}
