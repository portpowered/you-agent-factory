const committedReceipt = Object.freeze({ status: "committed" });

export class FactoryEmulatorAdvanceInProgressError extends Error {
  constructor() {
    super("Factory emulator is waiting for the current logical tick to be accepted");
    this.name = "FactoryEmulatorAdvanceInProgressError";
  }
}

export class FactoryEmulatorPendingTransactionError extends Error {
  constructor() {
    super("Factory emulator has a rejected logical tick that must be retried or reset");
    this.name = "FactoryEmulatorPendingTransactionError";
  }
}

export class FactoryEmulatorClosedError extends Error {
  constructor() {
    super("Factory emulator is closed");
    this.name = "FactoryEmulatorClosedError";
  }
}

/**
 * Creates a transport-neutral logical-tick emulator.
 *
 * State and calculated tick values are copied at the emulator boundary. This
 * keeps the sink write and the eventual commit paired to one immutable
 * calculation, even when the host retains or mutates its original values.
 */
export function createFactoryEmulator({ calculateClose, calculateTick, initialState, sink }) {
  if (typeof calculateTick !== "function") {
    throw new TypeError("calculateTick must be a function");
  }
  if (!sink || typeof sink.write !== "function") {
    throw new TypeError("sink must provide a write function");
  }

  let committedState = copy(initialState);
  let advanceInProgress = false;
  let closed = false;
  let pendingTick;
  let pendingCloseBatch;
  let pendingCloseBatchAccepted = false;
  let lastError;

  return Object.freeze({
    async advance() {
      if (advanceInProgress) {
        throw new FactoryEmulatorAdvanceInProgressError();
      }
      assertOpen(closed);
      if (pendingCloseBatch) {
        throw new FactoryEmulatorPendingTransactionError();
      }

      const calculation = pendingTick ?? calculateCalculation(calculateTick, committedState);
      return writeAndCommitTick(calculation);
    },
    async close() {
      if (advanceInProgress) {
        throw new FactoryEmulatorAdvanceInProgressError();
      }
      if (closed) {
        throw new FactoryEmulatorClosedError();
      }
      if (pendingTick) {
        throw new FactoryEmulatorPendingTransactionError();
      }

      if (!pendingCloseBatch) {
        if (typeof calculateClose !== "function") {
          throw new TypeError("calculateClose must be a function before closing the emulator");
        }
        pendingCloseBatch = calculateCloseBatch(calculateClose, committedState);
      }
      if (!pendingCloseBatchAccepted) {
        advanceInProgress = true;
        try {
          await sink.write(copy(pendingCloseBatch));
          pendingCloseBatchAccepted = true;
          lastError = undefined;
        } catch (error) {
          lastError = errorStatus("write", error);
          throw error;
        } finally {
          advanceInProgress = false;
        }
      }

      advanceInProgress = true;
      try {
        await sink.close();
        closed = true;
        lastError = undefined;
        return copy({ status: "closed", batch: pendingCloseBatch });
      } catch (error) {
        lastError = errorStatus("close", error);
        throw error;
      } finally {
        advanceInProgress = false;
      }
    },
    reset() {
      if (advanceInProgress) {
        throw new FactoryEmulatorAdvanceInProgressError();
      }
      if (closed) {
        throw new FactoryEmulatorClosedError();
      }
      if (pendingCloseBatch) {
        if (pendingCloseBatchAccepted) {
          throw new FactoryEmulatorPendingTransactionError();
        }
        pendingCloseBatch = undefined;
        lastError = undefined;
        return;
      }
      pendingTick = undefined;
      lastError = undefined;
    },
    pending() {
      return pendingTick ? copy(pendingTick.batch) : undefined;
    },
    status() {
      return copy({
        phase: closed ? "closed" : pendingCloseBatch ? "closing" : pendingTick ? "pending" : "open",
        lastError,
      });
    },
    state() {
      return copy(committedState);
    },
  });

  async function writeAndCommitTick(calculation) {
    advanceInProgress = true;
    try {
      await sink.write(copy(calculation.batch));
      committedState = calculation.state;
      pendingTick = undefined;
      lastError = undefined;
      return copy({ ...committedReceipt, batch: calculation.batch });
    } catch (error) {
      pendingTick = calculation;
      lastError = errorStatus("write", error);
      throw error;
    } finally {
      advanceInProgress = false;
    }
  }
}

function calculateCalculation(calculateTick, committedState) {
  const calculation = copy(calculateTick(copy(committedState)));
  assertCalculation(calculation);
  return calculation;
}

function calculateCloseBatch(calculateClose, committedState) {
  const batch = copy(calculateClose(copy(committedState)));
  if (!batch || !Array.isArray(batch.events)) {
    throw new TypeError("calculateClose must return a terminal lifecycle event batch");
  }
  return batch;
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

function assertOpen(closed) {
  if (closed) {
    throw new FactoryEmulatorClosedError();
  }
}

function errorStatus(operation, error) {
  return {
    operation,
    message: error instanceof Error ? error.message : String(error),
  };
}

function copy(value) {
  return structuredClone(value);
}
