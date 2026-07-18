import { defineDataError } from "./data-error.js";

export const FactoryEmulatorDurationError = defineDataError(
  "FactoryEmulatorDurationError",
  "INVALID_DURATION",
  {
    message: () => "Factory emulator duration must be a non-negative safe integer",
    details: (durationMs) => ({ durationMs }),
  },
);

export function validateDuration(durationMs) {
  if (!Number.isSafeInteger(durationMs) || durationMs < 0) {
    throw new FactoryEmulatorDurationError(durationMs);
  }
}

export function stateAtElapsed(scenario, state, virtualElapsedMs) {
  return {
    ...state,
    virtualElapsedMs,
    virtualTime: virtualTimeAt(scenario, virtualElapsedMs),
  };
}

export function virtualTimeAt(scenario, virtualElapsedMs) {
  const time = new Date(scenario.startAt).getTime() + virtualElapsedMs;
  if (!Number.isFinite(time)) {
    throw new FactoryEmulatorDurationError(virtualElapsedMs);
  }
  try {
    return new Date(time).toISOString();
  } catch {
    throw new FactoryEmulatorDurationError(virtualElapsedMs);
  }
}
