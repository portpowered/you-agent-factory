import {
  compareFactoryEvents,
  type FactoryDefinition,
  type FactoryEvent,
  type FactoryRecording,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";

import { type FactoryEventSink, FactoryEventSinkError } from "./event-sink.js";

export interface RecordingFactoryEventSinkOptions {
  /** Caller-owned recording metadata. The events collection is owned by the sink. */
  readonly recording: Omit<FactoryRecording, "events">;
  /** Maximum number of events retained. A batch that crosses this bound fails whole. */
  readonly maxEvents: number;
  /** Optional asynchronous boundary for backpressure or caller-controlled failure injection. */
  readonly beforeWrite?: (events: readonly FactoryEvent[]) => Promise<void>;
}

function cloneValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function eventFactoryName(event: FactoryEvent): string | undefined {
  const payload = event.payload as unknown;
  if (typeof payload !== "object" || payload === null) return undefined;
  const factory = (payload as { factory?: unknown }).factory;
  if (typeof factory !== "object" || factory === null) return undefined;
  const name = (factory as { name?: unknown }).name;
  return typeof name === "string" ? name : undefined;
}

function assertUniqueEventIds(events: readonly FactoryEvent[]): void {
  const seen = new Set<string>();
  for (const event of events) {
    if (seen.has(event.id)) {
      throw new FactoryEventSinkError(
        "duplicate_event_id",
        `Factory Event ID ${event.id} is already present in the recording.`,
      );
    }
    seen.add(event.id);
  }
}

function assertCanonicalOrder(events: readonly FactoryEvent[]): void {
  for (let index = 1; index < events.length; index += 1) {
    const previous = events[index - 1];
    const current = events[index];
    if (
      previous !== undefined &&
      current !== undefined &&
      compareFactoryEvents(previous, current) > 0
    ) {
      throw new FactoryEventSinkError(
        "invalid_event_order",
        `Factory Event ${current.id} is not in canonical recording order.`,
      );
    }
  }
}

function assertConsistentSessionIdentity(
  events: readonly FactoryEvent[],
): void {
  const sessionIds = new Set(
    events
      .map((event) => event.context.sessionId)
      .filter((sessionId): sessionId is string => sessionId !== undefined),
  );
  const hasMissingIdentity = events.some(
    (event) => event.context.sessionId === undefined,
  );
  if (sessionIds.size > 1 || (sessionIds.size === 1 && hasMissingIdentity)) {
    throw new FactoryEventSinkError(
      "mixed_session_identity",
      "Factory recording events must use one consistent Factory Session identity.",
    );
  }
}

function assertConsistentFactoryIdentity(
  factory: FactoryDefinition | undefined,
  events: readonly FactoryEvent[],
): void {
  const factoryNames = new Set<string>();
  if (factory?.name !== undefined) factoryNames.add(factory.name);
  for (const event of events) {
    const name = eventFactoryName(event);
    if (name !== undefined) factoryNames.add(name);
  }
  if (factoryNames.size > 1) {
    throw new FactoryEventSinkError(
      "mixed_factory_identity",
      "Factory recording metadata and topology events must use one Factory identity.",
    );
  }
}

function validateRecording(recording: FactoryRecording): void {
  assertUniqueEventIds(recording.events);
  assertCanonicalOrder(recording.events);
  assertConsistentFactoryIdentity(recording.factory, recording.events);
  assertConsistentSessionIdentity(recording.events);
  const result = safeParseFactoryRecording(recording);
  if (!result.success) {
    throw new FactoryEventSinkError(
      "invalid_recording",
      `Factory recording validation failed: ${result.issues[0]?.message ?? "unknown error"}`,
    );
  }
}

/**
 * Bounded, caller-owned canonical Factory recording. Concurrent writes are
 * processed in call order and each proposed full recording is validated before
 * the incoming batch becomes visible.
 */
export class RecordingFactoryEventSink implements FactoryEventSink {
  readonly #metadata: Omit<FactoryRecording, "events">;
  readonly #maxEvents: number;
  readonly #beforeWrite?: RecordingFactoryEventSinkOptions["beforeWrite"];
  #events: FactoryEvent[] = [];
  #acceptingWrites = true;
  #tail: Promise<void> = Promise.resolve();
  #closePromise: Promise<void> | undefined;

  constructor(options: RecordingFactoryEventSinkOptions) {
    if (!Number.isSafeInteger(options.maxEvents) || options.maxEvents < 1) {
      throw new FactoryEventSinkError(
        "invalid_capacity",
        "Recording sink maxEvents must be a positive safe integer.",
      );
    }
    this.#metadata = cloneValue(options.recording);
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

    const proposedBatch = cloneValue(events);
    const write = this.#tail.then(async () => {
      await this.#beforeWrite?.(cloneValue(proposedBatch));
      if (this.#events.length + proposedBatch.length > this.#maxEvents) {
        throw new FactoryEventSinkError(
          "capacity_exceeded",
          `The batch would exceed the configured ${this.#maxEvents}-event recording capacity.`,
        );
      }
      const proposedRecording = {
        ...this.#metadata,
        events: [...this.#events, ...proposedBatch],
      } satisfies FactoryRecording;
      validateRecording(proposedRecording);
      this.#events = proposedRecording.events;
    });
    this.#tail = write.catch(() => undefined);
    return write;
  }

  snapshot(): FactoryRecording {
    return cloneValue({ ...this.#metadata, events: this.#events });
  }

  close(): Promise<void> {
    if (this.#closePromise !== undefined) return this.#closePromise;
    this.#acceptingWrites = false;
    this.#closePromise = this.#tail;
    return this.#closePromise;
  }
}
