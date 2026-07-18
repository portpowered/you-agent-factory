const acceptedReceipt = Object.freeze({ status: "accepted" });
const closedReceipt = Object.freeze({ status: "closed" });

export class FactoryEventSinkClosedError extends Error {
  constructor() {
    super("Factory event sink is closed");
    this.name = "FactoryEventSinkClosedError";
  }
}

export class FactoryEventSinkCapacityError extends Error {
  constructor(limit, attempted) {
    super(
      `Factory event sink capacity ${limit} cannot accept ${attempted} more event${attempted === 1 ? "" : "s"}`,
    );
    this.name = "FactoryEventSinkCapacityError";
  }
}

export function createMemoryFactoryEventSink({ maxEvents }) {
  assertPositiveInteger(maxEvents, "maxEvents");
  const retainedBatches = [];
  let retainedEventCount = 0;
  let isClosed = false;

  return Object.freeze({
    maxEvents,
    async write(batch) {
      assertOpen(isClosed);
      const copiedBatch = copyBatch(batch);
      if (copiedBatch.events.length === 0) {
        return acceptedReceipt;
      }
      if (copiedBatch.events.length > maxEvents) {
        throw new FactoryEventSinkCapacityError(maxEvents, copiedBatch.events.length);
      }
      while (
        retainedBatches.length > 0 &&
        retainedEventCount + copiedBatch.events.length > maxEvents
      ) {
        retainedEventCount -= retainedBatches.shift().events.length;
      }
      retainedBatches.push(copiedBatch);
      retainedEventCount += copiedBatch.events.length;
      return acceptedReceipt;
    },
    async close() {
      isClosed = true;
      return closedReceipt;
    },
    batches() {
      return copyBatches(retainedBatches);
    },
  });
}

export function createFactoryRecordingSink({ sessionId, maxEvents }) {
  if (typeof sessionId !== "string" || sessionId.length === 0) {
    throw new TypeError("sessionId must be a non-empty string");
  }
  assertPositiveInteger(maxEvents, "maxEvents");

  const retainedEvents = [];
  let isClosed = false;

  return Object.freeze({
    sessionId,
    maxEvents,
    async write(batch) {
      assertOpen(isClosed);
      const copiedBatch = copyBatch(batch);
      assertRecordingSession(copiedBatch.events, sessionId);
      const attemptedCount = retainedEvents.length + copiedBatch.events.length;
      if (attemptedCount > maxEvents) {
        throw new FactoryEventSinkCapacityError(maxEvents, copiedBatch.events.length);
      }
      retainedEvents.push(...copiedBatch.events);
      return acceptedReceipt;
    },
    async close() {
      isClosed = true;
      return closedReceipt;
    },
    recording() {
      return structuredClone({
        schemaVersion: "agent-factory.recording.v1",
        sessionId,
        events: retainedEvents,
      });
    },
  });
}

function assertPositiveInteger(value, name) {
  if (!Number.isSafeInteger(value) || value < 1) {
    throw new RangeError(`${name} must be a positive safe integer`);
  }
}

function assertOpen(isClosed) {
  if (isClosed) {
    throw new FactoryEventSinkClosedError();
  }
}

function assertRecordingSession(events, sessionId) {
  if (events.some((event) => event.context.sessionId !== sessionId)) {
    throw new TypeError("recording sink batch has an event for another session");
  }
}

function copyBatch(batch) {
  return structuredClone(batch);
}

function copyBatches(batches) {
  return structuredClone(batches);
}
