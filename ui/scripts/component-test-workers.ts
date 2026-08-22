import { cpus } from "node:os";

export const componentTestMaxWorkersEnvironmentVariable =
  "UI_COMPONENT_TEST_MAX_WORKERS";
export const minimumComponentTestWorkers = 2;

type ComponentTestEnvironment = Record<string, string | undefined>;

export function detectLogicalCpuCount() {
  return cpus().length;
}

export function getComponentTestMaxWorkers(
  env: ComponentTestEnvironment = process.env,
  logicalCpuCount = detectLogicalCpuCount(),
) {
  const rawOverride = env[componentTestMaxWorkersEnvironmentVariable];
  if (rawOverride !== undefined) {
    const trimmedOverride = rawOverride.trim();
    const parsedOverride = Number(trimmedOverride);
    if (
      !/^\d+$/.test(trimmedOverride) ||
      !Number.isSafeInteger(parsedOverride) ||
      parsedOverride < 1
    ) {
      throw new Error(
        `Invalid ${componentTestMaxWorkersEnvironmentVariable} "${rawOverride}"; expected a positive integer (for example, 4).`,
      );
    }

    return parsedOverride;
  }

  const detectedWorkers = Number.isSafeInteger(logicalCpuCount)
    ? logicalCpuCount
    : 0;
  return Math.max(minimumComponentTestWorkers, detectedWorkers);
}

export function buildVitestComponentArgs({
  componentTests,
  env = process.env,
  forwardedArgs = [],
  logicalCpuCount = detectLogicalCpuCount(),
}: {
  componentTests: readonly string[];
  env?: ComponentTestEnvironment;
  forwardedArgs?: readonly string[];
  logicalCpuCount?: number;
}) {
  return [
    "run",
    "--config",
    "vitest.lanes.config.ts",
    "--project=dashboard-component",
    `--maxWorkers=${getComponentTestMaxWorkers(env, logicalCpuCount)}`,
    "--retry=0",
    ...forwardedArgs,
    ...componentTests,
  ];
}
