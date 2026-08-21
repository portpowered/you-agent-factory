const MINIMUM_RUNNER_JOBS = 2;

function parseLogicalCPUs(rawValue) {
	const value = String(rawValue ?? "").trim();
	if (!/^\d+$/.test(value)) {
		return 0;
	}

	const logicalCPUs = Number(value);
	return Number.isSafeInteger(logicalCPUs) && logicalCPUs > 0 ? logicalCPUs : 0;
}

export function resolveRunnerParallelism(rawLogicalCPUs) {
	const logicalCPUs = parseLogicalCPUs(rawLogicalCPUs);
	return {
		logicalCPUs,
		jobs: Math.max(MINIMUM_RUNNER_JOBS, logicalCPUs),
	};
}
