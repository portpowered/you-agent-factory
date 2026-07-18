import type { FactoryEvent } from "@you-agent-factory/client";

export type FactoryEventSinkErrorCode =
  | "capacity_exceeded"
  | "closed"
  | "duplicate_event_id"
  | "empty_batch"
  | "invalid_capacity"
  | "invalid_event_order"
  | "invalid_recording"
  | "mixed_factory_identity"
  | "mixed_session_identity";

export class FactoryEventSinkError extends Error {
  readonly code: FactoryEventSinkErrorCode;

  constructor(code: FactoryEventSinkErrorCode, message: string) {
    super(message);
    this.name = "FactoryEventSinkError";
    this.code = code;
  }
}

/** Caller-owned destination for one ordered, atomic Factory Event batch. */
export interface FactoryEventSink {
  write(events: readonly FactoryEvent[]): Promise<void>;
}

export interface MemoryFactoryEventSinkOptions {
  /** Maximum number of events retained. A batch that crosses this bound fails whole. */
  readonly maxEvents: number;
  /** Optional asynchronous boundary for backpressure or caller-controlled failure injection. */
  readonly beforeWrite?: (events: readonly FactoryEvent[]) => Promise<void>;
}

function cloneEvents(events: readonly FactoryEvent[]): FactoryEvent[] {
  return JSON.parse(JSON.stringify(events)) as FactoryEvent[];
}

/**
 * Bounded, caller-owned event history. Concurrent writes are processed in call
 * order, and one rejected write does not prevent a later write or retry.
 */
export class MemoryFactoryEventSink implements FactoryEventSink {
  readonly #maxEvents: number;
  readonly #beforeWrite?: MemoryFactoryEventSinkOptions["beforeWrite"];
  #events: FactoryEvent[] = [];
  #acceptingWrites = true;
  #tail: Promise<void> = Promise.resolve();
  #closePromise: Promise<void> | undefined;

  constructor(options: MemoryFactoryEventSinkOptions) {
    if (!Number.isSafeInteger(options.maxEvents) || options.maxEvents < 1) {
      throw new FactoryEventSinkError(
        "invalid_capacity",
        "Memory sink maxEvents must be a positive safe integer.",
      );
    }
    this.#maxEvents = options.maxEvents;
    this.#beforeWrite = options.beforeWrite;
  }

  write(events: readonly FactoryEvent[]): Promise<void> {
    if (!this.#acceptingWrites) {
      return Promise.reject(
        new FactoryEventSinkError("closed", "The event sink is closed."),
      );
    }
    if (events.length === 0) {
      return Promise.reject(
        new FactoryEventSinkError(
          "empty_batch",
          "Factory Event batches must be non-empty.",
        ),
      );
    }

    const proposedBatch = cloneEvents(events);
    const write = this.#tail.then(async () => {
      await this.#beforeWrite?.(cloneEvents(proposedBatch));
      if (this.#events.length + proposedBatch.length > this.#maxEvents) {
        throw new FactoryEventSinkError(
          "capacity_exceeded",
          `The batch would exceed the configured ${this.#maxEvents}-event capacity.`,
        );
      }
      this.#events = [...this.#events, ...proposedBatch];
    });
    this.#tail = write.catch(() => undefined);
    return write;
  }

  snapshot(): readonly FactoryEvent[] {
    return cloneEvents(this.#events);
  }

  close(): Promise<void> {
    if (this.#closePromise !== undefined) return this.#closePromise;
    this.#acceptingWrites = false;
    this.#closePromise = this.#tail;
    return this.#closePromise;
  }
}
