export const DEFAULT_FACTORY_EMULATOR_LIMITS = Object.freeze({
  maxCompletedDispatches: 1_000,
  maxEvents: 10_000,
  maxVirtualElapsedMs: 60 * 60 * 1_000,
  maxZeroDurationBatches: 1_000,
  maxSynchronousBatches: 100,
  maxSynchronousWorkItems: 100,
});

export const FACTORY_EMULATOR_LIMIT_HARD_CAPS = Object.freeze({
  maxCompletedDispatches: 100_000,
  maxEvents: 1_000_000,
  maxVirtualElapsedMs: 365 * 24 * 60 * 60 * 1_000,
  maxZeroDurationBatches: 100_000,
  maxSynchronousBatches: 10_000,
  maxSynchronousWorkItems: 10_000,
});

const limitNames = Object.freeze(Object.keys(DEFAULT_FACTORY_EMULATOR_LIMITS));
const minimumLimitValues = Object.freeze({
  maxCompletedDispatches: 1,
  // start always publishes INITIAL_STRUCTURE_REQUEST and RUN_REQUEST atomically.
  maxEvents: 2,
  maxVirtualElapsedMs: 1,
  maxZeroDurationBatches: 1,
  maxSynchronousBatches: 1,
  maxSynchronousWorkItems: 1,
});

/** Normalizes caller policy without reading environment or process-global state. */
export function normalizeFactoryEmulatorLimits(limits) {
  if (limits === undefined) {
    return { success: true, limits: { ...DEFAULT_FACTORY_EMULATOR_LIMITS } };
  }
  if (limits === null || typeof limits !== "object" || Array.isArray(limits)) {
    return invalid("/limits", "must be an object containing positive integer limits");
  }
  const unknown = Object.keys(limits).find((name) => !limitNames.includes(name));
  if (unknown !== undefined) {
    return invalid(`/limits/${unknown}`, "is not a supported emulator limit");
  }

  const normalized = { ...DEFAULT_FACTORY_EMULATOR_LIMITS };
  for (const name of limitNames) {
    const value = limits[name];
    if (value === undefined) {
      continue;
    }
    if (!Number.isSafeInteger(value) || value < minimumLimitValues[name]) {
      return invalid(
        `/limits/${name}`,
        `must be a safe integer no less than ${minimumLimitValues[name]}`,
      );
    }
    if (value > FACTORY_EMULATOR_LIMIT_HARD_CAPS[name]) {
      return invalid(
        `/limits/${name}`,
        `must not exceed the library hard cap ${FACTORY_EMULATOR_LIMIT_HARD_CAPS[name]}`,
      );
    }
    normalized[name] = value;
  }
  return { success: true, limits: normalized };
}

export function budgetExceededDiagnostic({
  configured,
  limit,
  observed,
  virtualTime,
  virtualElapsedMs,
}) {
  return {
    kind: "budget-exceeded",
    limit,
    configured,
    observed,
    virtualTime,
    virtualElapsedMs,
  };
}

export function zeroDurationCycleDiagnostic({
  configured,
  observed,
  virtualTime,
  virtualElapsedMs,
}) {
  return {
    kind: "zero-duration-cycle",
    limit: "zeroDurationBatches",
    configured,
    observed,
    virtualTime,
    virtualElapsedMs,
  };
}

export function synchronousWorkLimitDiagnostic({
  configured,
  observed,
  virtualTime,
  virtualElapsedMs,
}) {
  return {
    kind: "synchronous-work-limit",
    limit: "schedulerWorkItems",
    configured,
    observed,
    virtualTime,
    virtualElapsedMs,
  };
}

function invalid(path, message) {
  return {
    success: false,
    diagnostics: [{
      code: "INVALID_LIMIT_CONFIGURATION",
      path,
      message,
      expectation: "a supported safe integer within its documented minimum and hard cap",
    }],
  };
}
