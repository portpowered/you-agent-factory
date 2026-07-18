import type { FactoryEvent } from "@you-agent-factory/client";

/**
 * One ordered canonical Factory event batch submitted by an emulator logical
 * tick. This value contains data only, so hosts can structured-clone it across
 * process or worker boundaries.
 */
export interface FactoryEventBatch {
  readonly events: readonly FactoryEvent[];
}

/**
 * The only successful result of a batch write. A receipt is returned only when
 * every supplied event was accepted in its supplied order; partial acceptance
 * must reject the write instead of returning this receipt.
 */
export interface FactoryEventSinkWriteReceipt {
  readonly status: "accepted";
}

/**
 * The only successful result of an explicit sink close. Closing does not
 * implicitly accept a pending batch; callers must await its write first.
 */
export interface FactoryEventSinkCloseReceipt {
  readonly status: "closed";
}

/**
 * Caller-owned asynchronous destination for canonical Factory events.
 *
 * `write` is the backpressure boundary: resolution means the entire ordered
 * batch was accepted, while rejection means none of the batch was accepted.
 * `close` is explicit and asynchronous so a host can observe either its
 * successful completion or rejection. The contract deliberately has no
 * subscription, replay, transport, browser, or dashboard dependency.
 */
export interface FactoryEventSink {
  write(batch: FactoryEventBatch): Promise<FactoryEventSinkWriteReceipt>;
  close(): Promise<FactoryEventSinkCloseReceipt>;
}
