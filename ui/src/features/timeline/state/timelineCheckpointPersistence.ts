import type { FactoryEventReconnectCursor } from "../../../api/events";
import type { FactoryTimelineCheckpoint } from "./timeline/storeState";
import type { ReplayWorldState } from "./timeline/types";

const CHECKPOINT_SCHEMA_VERSION_GUARDED = 2;
const CHECKPOINT_DB_NAME = "agentFactoryTimelineCheckpoints";
const CHECKPOINT_DB_VERSION = 1;
const CHECKPOINT_STORE_NAME = "checkpoints";
const MAX_COMPACT_TEXT_CHARS = 512;

interface TimelineCheckpointEnvelope {
  checkpoint: PersistedTimelineCheckpoint;
  schemaVersion: number;
  sessionID: string;
  streamIdentity?: TimelineCheckpointStreamIdentity;
}

interface PersistedTimelineCheckpoint {
  afterEventId?: string;
  afterSequence?: number;
  replayState: ReplayWorldState;
  selectedTick: number;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
}

export interface TimelineCheckpointStreamIdentity {
  backendScopeID: string;
  factorySessionID: string;
  streamGenerationID: string;
}

interface IndexedDBLike {
  open: IDBFactory["open"];
}

function openCheckpointDatabase(
  indexedDB: IndexedDBLike,
): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(CHECKPOINT_DB_NAME, CHECKPOINT_DB_VERSION);

    request.onupgradeneeded = () => {
      const database = request.result;
      if (!database.objectStoreNames.contains(CHECKPOINT_STORE_NAME)) {
        database.createObjectStore(CHECKPOINT_STORE_NAME, {
          keyPath: "sessionID",
        });
      }
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
    request.onblocked = () =>
      reject(new Error("timeline checkpoint IndexedDB upgrade blocked"));
  });
}

function requestToPromise<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

async function writeIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  envelope: TimelineCheckpointEnvelope,
): Promise<void> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    await requestToPromise(store.put(envelope));
  } finally {
    database.close();
  }
}

async function readIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  sessionID: string,
): Promise<TimelineCheckpointEnvelope | null> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const result = await requestToPromise<
      TimelineCheckpointEnvelope | undefined
    >(store.get(sessionID));
    return result ?? null;
  } finally {
    database.close();
  }
}

async function deleteIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  sessionID: string,
): Promise<void> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    await requestToPromise(store.delete(sessionID));
  } finally {
    database.close();
  }
}

function compactText(value: string): string {
  if (value.length <= MAX_COMPACT_TEXT_CHARS) {
    return value;
  }
  // hardcoded-ui-copy-exception: non-product-diagnostic
  return `${value.slice(0, MAX_COMPACT_TEXT_CHARS)}\n\n[checkpoint truncated ${value.length - MAX_COMPACT_TEXT_CHARS} chars]`;
}

function compactReplayState(state: ReplayWorldState): ReplayWorldState {
  const compacted = structuredClone(state);

  for (const [textBlobID, value] of Object.entries(compacted.textBlobsByID)) {
    compacted.textBlobsByID[textBlobID] = compactText(value);
  }

  return compacted;
}

function buildPersistedCheckpoint(
  checkpoint: FactoryTimelineCheckpoint,
): PersistedTimelineCheckpoint {
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    replayState: compactReplayState(checkpoint.replayState),
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  };
}

function hydrateCheckpoint(
  checkpoint: PersistedTimelineCheckpoint,
): FactoryTimelineCheckpoint {
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
    replayState: checkpoint.replayState,
    selectedTick: checkpoint.selectedTick,
    syncIdentity: checkpoint.syncIdentity,
  };
}

function parseStoredCheckpoint(
  envelope: TimelineCheckpointEnvelope,
  sessionID: string | null,
  expectedIdentity: TimelineCheckpointStreamIdentity | null,
): FactoryTimelineCheckpoint | null {
  if (
    envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
    envelope.sessionID !== sessionID ||
    !matchesStreamIdentity(envelope.streamIdentity, expectedIdentity) ||
    !envelope.checkpoint?.replayState
  ) {
    return null;
  }
  return hydrateCheckpoint(envelope.checkpoint);
}

function normalizeStreamIdentity(
  identity: TimelineCheckpointStreamIdentity | null | undefined,
): TimelineCheckpointStreamIdentity | null {
  if (!identity) {
    return null;
  }
  const backendScopeID = identity.backendScopeID.trim();
  const factorySessionID = identity.factorySessionID.trim();
  const streamGenerationID = identity.streamGenerationID.trim();
  if (
    backendScopeID === "" ||
    factorySessionID === "" ||
    streamGenerationID === ""
  ) {
    return null;
  }
  return {
    backendScopeID,
    factorySessionID,
    streamGenerationID,
  };
}

function matchesStreamIdentity(
  actual: TimelineCheckpointStreamIdentity | null | undefined,
  expected: TimelineCheckpointStreamIdentity | null,
): boolean {
  const normalizedActual = normalizeStreamIdentity(actual);
  return (
    normalizedActual != null &&
    expected != null &&
    normalizedActual.backendScopeID === expected.backendScopeID &&
    normalizedActual.factorySessionID === expected.factorySessionID &&
    normalizedActual.streamGenerationID === expected.streamGenerationID
  );
}

export async function persistTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
  checkpoint: FactoryTimelineCheckpoint | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  const normalizedStreamIdentity = normalizeStreamIdentity(streamIdentity);
  if (!indexedDB || !sessionID || !checkpoint || !normalizedStreamIdentity) {
    return;
  }

  const envelope = {
    checkpoint: buildPersistedCheckpoint(checkpoint),
    schemaVersion: CHECKPOINT_SCHEMA_VERSION_GUARDED,
    sessionID,
    streamIdentity: normalizedStreamIdentity,
  } satisfies TimelineCheckpointEnvelope;

  try {
    await writeIndexedCheckpoint(indexedDB, envelope);
  } catch {
    await deleteIndexedCheckpoint(indexedDB, sessionID).catch(() => {});
  }
}

export async function clearTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
): Promise<void> {
  if (!indexedDB || !sessionID) {
    return;
  }

  await deleteIndexedCheckpoint(indexedDB, sessionID).catch(() => {});
}

export async function readTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<FactoryTimelineCheckpoint | null> {
  const normalizedStreamIdentity = normalizeStreamIdentity(streamIdentity);
  if (!indexedDB || !sessionID || !normalizedStreamIdentity) {
    return null;
  }

  try {
    const envelope = await readIndexedCheckpoint(indexedDB, sessionID);
    if (envelope) {
      const checkpoint = parseStoredCheckpoint(
        envelope,
        sessionID,
        normalizedStreamIdentity,
      );
      if (!checkpoint) {
        await deleteIndexedCheckpoint(indexedDB, sessionID).catch(() => {});
      }
      return checkpoint;
    }
  } catch {
    await deleteIndexedCheckpoint(indexedDB, sessionID).catch(() => {});
  }

  return null;
}

export function reconnectCursorFromCheckpoint(
  checkpoint: FactoryTimelineCheckpoint | null,
): FactoryEventReconnectCursor | undefined {
  if (!checkpoint) {
    return undefined;
  }
  if (!checkpoint.afterEventId && checkpoint.afterSequence == null) {
    return undefined;
  }
  return {
    afterEventId: checkpoint.afterEventId,
    afterSequence: checkpoint.afterSequence,
  };
}
