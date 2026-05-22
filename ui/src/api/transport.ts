export interface APIErrorPayload<TTarget = never> {
  code?: string;
  message?: string;
  targets?: TTarget[];
}

export async function readAPIResponseBody(
  response: Response,
): Promise<unknown> {
  const rawBody = await response.text();
  if (rawBody.length === 0) {
    return null;
  }

  try {
    return JSON.parse(rawBody) as unknown;
  } catch {
    return rawBody;
  }
}

export function isAPIRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function extractAPIErrorPayload<TTarget>(
  value: unknown,
  options: {
    isTarget?: (value: unknown) => value is TTarget;
  } = {},
): APIErrorPayload<TTarget> | null {
  if (!isAPIRecord(value)) {
    return null;
  }

  const code = typeof value.code === "string" ? value.code : undefined;
  const message = typeof value.message === "string" ? value.message : undefined;

  if (code === undefined && message === undefined) {
    return null;
  }

  return {
    code,
    message,
    targets: readAPIErrorTargets(value.targets, options.isTarget),
  };
}

function readAPIErrorTargets<TTarget>(
  value: unknown,
  isTarget?: (value: unknown) => value is TTarget,
): TTarget[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }

  if (typeof isTarget !== "function") {
    return value as TTarget[];
  }

  const targets = value.filter((entry): entry is TTarget => isTarget(entry));
  return targets.length > 0 ? targets : undefined;
}
