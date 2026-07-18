import type { FactoryRecording } from "@you-agent-factory/client";

import type {
  FactoryEventBatch,
  FactoryEventSink,
} from "./contracts.js";

/** Raised when a caller writes to a helper after it has closed. */
export declare class FactoryEventSinkClosedError extends Error {}

/** Raised when a batch cannot fit without exceeding a helper's declared bound. */
export declare class FactoryEventSinkCapacityError extends Error {}

/**
 * The maximum number of events a memory sink retains. Batches remain atomic:
 * an oversized batch is rejected and older complete batches are discarded
 * before a later accepted batch is retained.
 */
export interface MemoryFactoryEventSinkOptions {
  readonly maxEvents: number;
}

/**
 * A bounded in-memory sink. `batches` returns a detached snapshot, not the
 * values retained by the sink.
 */
export interface MemoryFactoryEventSink extends FactoryEventSink {
  readonly maxEvents: number;
  batches(): readonly FactoryEventBatch[];
}

/** Creates a bounded, detached in-memory event sink. */
export declare function createMemoryFactoryEventSink(
  options: MemoryFactoryEventSinkOptions,
): MemoryFactoryEventSink;

/** The fixed metadata and capacity of one finite Factory recording. */
export interface FactoryRecordingSinkOptions {
  readonly sessionId: string;
  readonly maxEvents: number;
}

/**
 * A bounded sink that produces one finite recording. `recording` returns a
 * detached snapshot and remains readable after close.
 */
export interface FactoryRecordingSink extends FactoryEventSink {
  readonly sessionId: string;
  readonly maxEvents: number;
  recording(): FactoryRecording;
}

/** Creates a bounded sink for one caller-owned Factory recording. */
export declare function createFactoryRecordingSink(
  options: FactoryRecordingSinkOptions,
): FactoryRecordingSink;
