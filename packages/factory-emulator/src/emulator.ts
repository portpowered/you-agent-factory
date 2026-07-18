import type { FactoryEventBatch, FactoryEventSink } from "./contracts.js";

/** Raised when an advancing command arrives before the prior tick is accepted. */
export declare class FactoryEmulatorAdvanceInProgressError extends Error {}

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

/** Dependencies and initial state for one caller-owned emulator session. */
export interface FactoryEmulatorOptions<State> {
  readonly calculateTick: FactoryEmulatorTickCalculator<State>;
  readonly initialState: State;
  readonly sink: FactoryEventSink;
}

/** Returned only after the matching sink write accepts the entire batch. */
export interface FactoryEmulatorAdvanceReceipt {
  readonly status: "committed";
  readonly batch: FactoryEventBatch;
}

/**
 * A caller-owned logical-tick runtime.
 *
 * `advance` computes one full batch, awaits the asynchronous sink write, and
 * commits the calculated state only after that write resolves. A second
 * advancing command while the write is unresolved rejects explicitly, so it
 * cannot calculate or commit a later tick ahead of the pending batch.
 * `state` returns a detached snapshot of the latest committed state.
 */
export interface FactoryEmulator<State> {
  advance(): Promise<FactoryEmulatorAdvanceReceipt>;
  state(): State;
}

/** Creates an emulator with explicit state, scheduling, and sink ownership. */
export declare function createFactoryEmulator<State>(
  options: FactoryEmulatorOptions<State>,
): FactoryEmulator<State>;
