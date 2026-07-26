export const defaultComponentTestMaxDurationMs = 150_000;

export function getComponentTestMaxDurationMs(
  env: Record<string, string | undefined> = process.env,
) {
  const raw = env.UI_COMPONENT_MAX_DURATION_MS?.trim();
  if (!raw) {
    return defaultComponentTestMaxDurationMs;
  }

  const durationMs = Number(raw);
  if (!Number.isFinite(durationMs) || durationMs <= 0) {
    throw new Error(
      `Invalid UI_COMPONENT_MAX_DURATION_MS "${raw}"; expected a positive number`,
    );
  }

  return durationMs;
}

export function formatComponentDuration(durationMs: number) {
  return `${(durationMs / 1000).toFixed(2)}s`;
}

export function assertComponentTestDuration(
  durationMs: number,
  maxDurationMs: number,
) {
  if (durationMs > maxDurationMs) {
    throw new Error(
      `Component tests exceeded the ${formatComponentDuration(maxDurationMs)} wall-clock budget: ${formatComponentDuration(durationMs)}`,
    );
  }
}
