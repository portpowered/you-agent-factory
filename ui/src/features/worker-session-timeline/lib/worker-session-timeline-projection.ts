import {
  categoryForKind,
  projectAttempt,
  projectContinuation,
  projectFailure,
  projectIdentity,
  projectMessage,
  projectProgress,
  projectReasoning,
  projectTool,
  projectUsage,
  terminalOutcomeFor,
} from "./worker-session-timeline-projection-details";
import {
  asObject,
  genericMetadata,
  normalizedToken,
  optionalString,
} from "./worker-session-timeline-projection-helpers";
import type {
  WorkerSessionEventRecord,
  WorkerSessionTimelineEntry,
  WorkerTimelineCanonicalSource,
  WorkerTimelineJSONObject,
} from "./worker-session-timeline-projection-types";

export type {
  WorkerSessionEventRecord,
  WorkerSessionTimelineEntry,
} from "./worker-session-timeline-projection-types";

const WORKER_DRAFT_SCHEMA_ID = "workers.draft.v1";

const LEGACY_SESSION_SCHEMA_IDS = new Set([
  "worker_session.lifecycle",
  "worker_session.lineage",
  "worker_session.attempt",
  "worker_session.started",
  "worker_session.running",
  "worker_session.completed",
  "worker_session.failed",
  "worker_session.canceled",
  "worker_session.cancelled",
]);

/**
 * Returns a deterministic key for one canonical record. The source tuple is
 * encoded as separate components so source IDs containing punctuation remain
 * unambiguous and a duplicate source identity cannot silently replace another
 * record in a rendered list.
 */
export function workerSessionTimelineEntryKey(
  record: Pick<
    WorkerSessionEventRecord,
    "position" | "sourceType" | "sourceId" | "sourceSequence" | "sourceEventId"
  >,
): string {
  return [
    record.position,
    record.sourceType,
    record.sourceId,
    record.sourceSequence,
    record.sourceEventId,
  ]
    .map((part) => encodeURIComponent(String(part)))
    .join(":");
}

/**
 * Projects canonical Worker Session records into provider-neutral timeline
 * entries. The input is never mutated and each input record produces exactly
 * one entry, including records with an unknown schema. Snapshot and delta
 * payloads retain their canonical representation and item/block identities so
 * a later view can combine them only according to the source schema rather
 * than guessing from provider transcripts or current component state.
 */
export function projectWorkerSessionTimeline(
  records: readonly WorkerSessionEventRecord[],
): WorkerSessionTimelineEntry[] {
  return records
    .map((record) => projectWorkerSessionTimelineEntry(record))
    .sort(compareTimelineEntries);
}

/** Projects one canonical record without consulting any external state. */
export function projectWorkerSessionTimelineEntry(
  record: WorkerSessionEventRecord,
): WorkerSessionTimelineEntry {
  const canonical = canonicalSource(record);
  const base: WorkerSessionTimelineEntry = {
    key: workerSessionTimelineEntryKey(record),
    canonical,
    kind: "UNKNOWN",
    phase: "UNKNOWN",
    category: "generic",
  };

  const decoded = decodeKnownRecord(record);
  if (decoded === undefined) {
    return { ...base, generic: genericMetadata(record) };
  }

  const kind = normalizedToken(decoded.kind) ?? "UNKNOWN";
  const phase = normalizedToken(decoded.phase) ?? "UNKNOWN";
  const entry: WorkerSessionTimelineEntry = {
    ...base,
    kind,
    phase,
    category: categoryForKind(kind),
  };
  const payload = decoded.payload;
  const envelope = decoded.envelope;
  const itemId = optionalString(envelope?.itemId);
  const parentItemId = optionalString(envelope?.parentItemId);
  if (itemId !== undefined) {
    entry.itemId = itemId;
  }
  if (parentItemId !== undefined) {
    entry.parentItemId = parentItemId;
  }

  const usage = projectUsage(payload);
  const identity = projectIdentity(envelope, payload, usage);
  const attempt = projectAttempt(envelope, payload);
  const continuation = projectContinuation(payload);
  const failure = projectFailure(kind, payload);
  if (identity !== undefined) {
    entry.identity = identity;
  }
  if (attempt !== undefined) {
    entry.attempt = attempt;
  }
  if (continuation !== undefined) {
    entry.continuation = continuation;
  }
  if (usage !== undefined) {
    entry.usage = usage;
  }
  if (failure !== undefined) {
    entry.failure = failure;
  }

  switch (kind) {
    case "MESSAGE": {
      const message = projectMessage(payload);
      if (message !== undefined) {
        entry.message = message;
      }
      break;
    }
    case "REASONING": {
      const reasoning = projectReasoning(phase, payload);
      if (reasoning !== undefined) {
        entry.reasoning = reasoning;
      }
      break;
    }
    case "PROGRESS": {
      const progress = projectProgress(payload);
      if (progress !== undefined) {
        entry.progress = progress;
      }
      break;
    }
    case "TOOL": {
      const tool = projectTool(payload);
      if (tool !== undefined) {
        entry.tool = tool;
      }
      break;
    }
    default:
      break;
  }

  const terminalOutcome = terminalOutcomeFor(kind, phase, payload);
  if (terminalOutcome !== undefined) {
    const status = optionalString(payload.status);
    entry.terminal = {
      outcome: terminalOutcome,
      ...(status !== undefined ? { status } : {}),
      ...(failure !== undefined ? { failure } : {}),
    };
  }
  return entry;
}

interface DecodedKnownRecord {
  kind?: unknown;
  phase?: unknown;
  payload: WorkerTimelineJSONObject;
  envelope?: WorkerTimelineJSONObject;
}

function decodeKnownRecord(
  record: WorkerSessionEventRecord,
): DecodedKnownRecord | undefined {
  if (record.schemaId === WORKER_DRAFT_SCHEMA_ID) {
    const envelope = asObject(record.payload);
    if (envelope === undefined) {
      return { payload: {} };
    }
    return {
      kind: envelope.kind,
      phase: envelope.phase,
      payload: asObject(envelope.payload) ?? {},
      envelope,
    };
  }
  if (!LEGACY_SESSION_SCHEMA_IDS.has(record.schemaId)) {
    return undefined;
  }
  const payload = asObject(record.payload) ?? {};
  return {
    kind: "SESSION",
    phase: legacySessionPhase(record.schemaId, payload),
    payload,
  };
}

function legacySessionPhase(
  schemaId: string,
  payload: WorkerTimelineJSONObject,
): string {
  switch (schemaId.slice("worker_session.".length)) {
    case "started":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "STARTED";
    case "running":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "UPDATED";
    case "completed":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "COMPLETED";
    case "failed":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "FAILED";
    case "canceled":
    case "cancelled":
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return "CANCELED";
    default:
      // hardcoded-ui-copy-exception: non-product-diagnostic
      return optionalString(payload.phase) ?? "UPDATED";
  }
}

function canonicalSource(
  record: WorkerSessionEventRecord,
): WorkerTimelineCanonicalSource {
  const workerSessionId = optionalString(record.cursor.workerSessionId);
  const streamGenerationId = optionalString(record.cursor.streamGenerationId);
  return {
    position: record.position,
    cursor: {
      position: record.cursor.position,
      ...(workerSessionId !== undefined ? { workerSessionId } : {}),
      ...(streamGenerationId !== undefined ? { streamGenerationId } : {}),
    },
    sourceType: record.sourceType,
    sourceId: record.sourceId,
    sourceSequence: record.sourceSequence,
    sourceEventId: record.sourceEventId,
    schemaId: record.schemaId,
  };
}

function compareTimelineEntries(
  left: WorkerSessionTimelineEntry,
  right: WorkerSessionTimelineEntry,
): number {
  const positionDifference = left.canonical.position - right.canonical.position;
  if (positionDifference !== 0) {
    return positionDifference;
  }
  const sourceSequenceDifference =
    left.canonical.sourceSequence - right.canonical.sourceSequence;
  if (sourceSequenceDifference !== 0) {
    return sourceSequenceDifference;
  }
  return left.key.localeCompare(right.key);
}
