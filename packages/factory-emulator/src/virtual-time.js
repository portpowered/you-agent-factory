export class FactoryEmulatorDurationError extends RangeError {
  constructor(durationMs) {
    super("Factory emulator duration must be a non-negative safe integer");
    this.name = "FactoryEmulatorDurationError";
    this.code = "INVALID_DURATION";
    this.durationMs = durationMs;
  }
}

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
