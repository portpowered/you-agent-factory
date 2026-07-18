import type { FactoryEventBatch, FactoryEventSink } from "./contracts.js";

/** Raised when an advancing command arrives before the prior tick is accepted. */
export declare class FactoryEmulatorAdvanceInProgressError extends Error {}

/** Raised when a rejected tick must be retried or explicitly reset first. */
export declare class FactoryEmulatorPendingTransactionError extends Error {}

/** Raised when a command is issued after the emulator has closed. */
export declare class FactoryEmulatorClosedError extends Error {}

/**
 * The complete result of calculating one logical scheduler step. The emulator
 * writes `batch` before it makes `state` visible as its next committed state.
 */
export interface FactoryEmulatorTick<State> {
  readonly batch: FactoryEventBatch;
  readonly state: State;
}

/** Computes one complete logical tick from a detached committed-state snapshot. */
export type FactoryEmulatorTickCalculator<State> = (
  state: State,
) => FactoryEmulatorTick<State>;

/** Builds the terminal lifecycle event batch written before the sink closes. */
export type FactoryEmulatorCloseCalculator<State> = (
  state: State,
) => FactoryEventBatch;

/** Dependencies and initial state for one caller-owned emulator session. */
export interface FactoryEmulatorOptions<State> {
  /** Required when callers use `close`; returns the terminal lifecycle batch. */
  readonly calculateClose?: FactoryEmulatorCloseCalculator<State>;
  readonly calculateTick: FactoryEmulatorTickCalculator<State>;
  readonly initialState: State;
  readonly sink: FactoryEventSink;
}

/** Returned only after the matching sink write accepts the entire batch. */
export interface FactoryEmulatorAdvanceReceipt {
  readonly status: "committed";
  readonly batch: FactoryEventBatch;
}

/** The result returned after a terminal batch and sink close both succeed. */
export interface FactoryEmulatorCloseReceipt {
  readonly status: "closed";
  readonly batch: FactoryEventBatch;
}

/** Observable retry and lifecycle status for one emulator session. */
export interface FactoryEmulatorStatus {
  readonly phase: "open" | "pending" | "closing" | "closed";
  readonly lastError?: {
    readonly operation: "write" | "close";
    readonly message: string;
  };
}

/**
 * A caller-owned logical-tick runtime.
 *
 * `advance` computes one full batch, awaits the asynchronous sink write, and
 * commits the calculated state only after that write resolves. A second
 * advancing command while the write is unresolved rejects explicitly, so it
 * cannot calculate or commit a later tick ahead of the pending batch.
 * A rejected write retains the calculated batch unchanged; the next `advance`
 * retries it before new work is calculated, while `reset` explicitly discards
 * it. `close` retries its caller-calculated terminal lifecycle batch unchanged
 * after rejection, then closes the sink. It cannot bypass a rejected tick.
 * `state` returns a detached snapshot of the latest committed state.
 */
export interface FactoryEmulator<State> {
  advance(): Promise<FactoryEmulatorAdvanceReceipt>;
  close(): Promise<FactoryEmulatorCloseReceipt>;
  reset(): void;
  pending(): FactoryEventBatch | undefined;
  status(): FactoryEmulatorStatus;
  state(): State;
}

/** Creates an emulator with explicit state, scheduling, and sink ownership. */
export declare function createFactoryEmulator<State>(
  options: FactoryEmulatorOptions<State>,
): FactoryEmulator<State>;
