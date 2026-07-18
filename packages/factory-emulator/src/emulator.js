const committedReceipt = Object.freeze({ status: "committed" });

export class FactoryEmulatorAdvanceInProgressError extends Error {
  constructor() {
    super("Factory emulator is waiting for the current logical tick to be accepted");
    this.name = "FactoryEmulatorAdvanceInProgressError";
  }
}

/**
 * Creates a transport-neutral logical-tick emulator.
 *
 * State and calculated tick values are copied at the emulator boundary. This
 * keeps the sink write and the eventual commit paired to one immutable
 * calculation, even when the host retains or mutates its original values.
 */
export function createFactoryEmulator({ calculateTick, initialState, sink }) {
  if (typeof calculateTick !== "function") {
    throw new TypeError("calculateTick must be a function");
  }
  if (!sink || typeof sink.write !== "function") {
    throw new TypeError("sink must provide a write function");
  }

  let committedState = copy(initialState);
  let advanceInProgress = false;

  return Object.freeze({
    async advance() {
      if (advanceInProgress) {
        throw new FactoryEmulatorAdvanceInProgressError();
      }

      const calculation = copy(calculateTick(copy(committedState)));
      assertCalculation(calculation);
      advanceInProgress = true;
      try {
        await sink.write(copy(calculation.batch));
        committedState = calculation.state;
        return copy({ ...committedReceipt, batch: calculation.batch });
      } finally {
        advanceInProgress = false;
      }
    },
    state() {
      return copy(committedState);
    },
  });
}

function assertCalculation(calculation) {
  if (!calculation || typeof calculation !== "object") {
    throw new TypeError("calculateTick must return a logical tick calculation");
  }
  if (!calculation.batch || !Array.isArray(calculation.batch.events)) {
    throw new TypeError("logical tick calculation batch must contain an events array");
  }
  if (!("state" in calculation)) {
    throw new TypeError("logical tick calculation must contain the calculated state");
  }
}

function copy(value) {
  return structuredClone(value);
}
