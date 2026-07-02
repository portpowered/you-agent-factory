import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../api/session-routing";
import {
  identityMismatchDiagnostic,
  recordSessionPersistenceInvalidation,
  userClearedSessionsDiagnostic,
  type SessionPersistenceIdentityScope,
} from "../../dashboard/public/session-persistence-diagnostics";
import {
  normalizeStreamDerivedCacheIdentity,
  streamDerivedCheckpointStorageKey,
  type StreamDerivedCacheIdentity,
} from "../lib/stream-derived-cache-identity";
import type { FactoryTimelineCheckpoint } from "./timeline/storeState";
import type { ReplayWorldState } from "./timeline/types";

const CHECKPOINT_SCHEMA_VERSION_GUARDED = 3;
const CHECKPOINT_DB_NAME = "agentFactoryTimelineCheckpoints";
const CHECKPOINT_DB_VERSION = 3;
const CHECKPOINT_STORE_NAME = "checkpoints";
const MAX_COMPACT_TEXT_CHARS = 512;

export type TimelineCheckpointStreamIdentity = StreamDerivedCacheIdentity;

interface TimelineCheckpointEnvelope {
  checkpoint: PersistedTimelineCheckpoint;
  schemaVersion: number;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity;
}

function matchesStoredCheckpointFactorySessionID(
  envelope: TimelineCheckpointEnvelope,
  factorySessionID: string,
): boolean {
  const requestedSessionID = factorySessionID.trim();
  const storedFactorySessionID =
    envelope.streamIdentity?.factorySessionID?.trim() ?? "";
  if (storedFactorySessionID === requestedSessionID) {
    return true;
  }
  return (
    isDefaultFactorySessionID(requestedSessionID) &&
    storedFactorySessionID !== "" &&
    !isDefaultFactorySessionID(storedFactorySessionID)
  );
}

interface PersistedTimelineCheckpoint {
  afterEventId?: string;
  afterSequence?: number;
  replayState: ReplayWorldState;
  selectedTick: number;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
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
      if (database.objectStoreNames.contains(CHECKPOINT_STORE_NAME)) {
        if (typeof database.deleteObjectStore === "function") {
          database.deleteObjectStore(CHECKPOINT_STORE_NAME);
        }
      }
      if (!database.objectStoreNames.contains(CHECKPOINT_STORE_NAME)) {
        database.createObjectStore(CHECKPOINT_STORE_NAME, {
          keyPath: "storageKey",
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
  storageKey: string,
): Promise<TimelineCheckpointEnvelope | null> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const result = await requestToPromise<
      TimelineCheckpointEnvelope | undefined
    >(store.get(storageKey));
    return result ?? null;
  } finally {
    database.close();
  }
}

async function deleteIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  storageKey: string,
): Promise<void> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    await requestToPromise(store.delete(storageKey));
  } finally {
    database.close();
  }
}

export interface PersistedTimelineCheckpointPeek {
  checkpoint: FactoryTimelineCheckpoint;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

async function listIndexedCheckpoints(
  indexedDB: IndexedDBLike,
): Promise<TimelineCheckpointEnvelope[]> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const result = await requestToPromise<TimelineCheckpointEnvelope[]>(
      store.getAll(),
    );
    return result ?? [];
  } finally {
    database.close();
  }
}

export async function peekPersistedTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
): Promise<PersistedTimelineCheckpointPeek | null> {
  const normalizedSessionID = sessionID?.trim();
  if (!indexedDB || !normalizedSessionID) {
    return null;
  }

  try {
    const envelopes = await listIndexedCheckpoints(indexedDB);
    const envelope = envelopes.find((candidate) =>
      matchesStoredCheckpointFactorySessionID(candidate, normalizedSessionID),
    );
    if (
      !envelope ||
      envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
      !envelope.checkpoint?.replayState
    ) {
      return null;
    }

    return {
      checkpoint: hydrateCheckpoint(envelope.checkpoint),
      storageKey: envelope.storageKey,
      streamIdentity: normalizeStreamDerivedCacheIdentity(
        envelope.streamIdentity,
      ),
    };
  } catch {
    return null;
  }
}

export async function clearTimelineCheckpointsForSession(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
): Promise<void> {
  const normalizedSessionID = sessionID?.trim();
  if (!indexedDB || !normalizedSessionID) {
    return;
  }

  try {
    const envelopes = await listIndexedCheckpoints(indexedDB);
    const storageKeys = envelopes
      .filter((envelope) =>
        matchesStoredCheckpointFactorySessionID(
          envelope,
          normalizedSessionID,
        ),
      )
      .map((envelope) => envelope.storageKey)
      .filter((storageKey) => storageKey.trim() !== "");

    await Promise.all(
      storageKeys.map((storageKey) =>
        deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {}),
      ),
    );
  } catch {
    // Best-effort cleanup for stale reconnect state.
  }
}

export async function clearTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
  options: { requestedSessionID?: string; userInitiated?: boolean } = {},
): Promise<void> {
  const storageKey = checkpointStorageKey(streamIdentity);
  if (!indexedDB || !storageKey) {
    return;
  }
  if (options.userInitiated && streamIdentity && options.requestedSessionID) {
    recordSessionPersistenceInvalidation(
      userClearedSessionsDiagnostic(
        persistenceScopeFromTimelineIdentity(streamIdentity),
        options.requestedSessionID,
      ),
    );
  }
  await deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {});
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
  expectedIdentity: TimelineCheckpointStreamIdentity | null,
): FactoryTimelineCheckpoint | null {
  if (
    envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
    !envelope.checkpoint?.replayState
  ) {
    return null;
  }
  if (!matchesStreamIdentity(envelope.streamIdentity, expectedIdentity)) {
    recordCheckpointIdentityMismatch(
      envelope.streamIdentity,
      expectedIdentity,
      envelope.streamIdentity?.factorySessionID ?? null,
    );
    return null;
  }
  return hydrateCheckpoint(envelope.checkpoint);
}

function recordCheckpointIdentityMismatch(
  actual: TimelineCheckpointStreamIdentity | null | undefined,
  expected: TimelineCheckpointStreamIdentity | null,
  sessionID: string | null,
): void {
  if (!actual || !expected || !sessionID) {
    return;
  }
  const diagnostic = identityMismatchDiagnostic(
    persistenceScopeFromTimelineIdentity(actual),
    persistenceScopeFromTimelineIdentity(expected),
    sessionID,
  );
  if (diagnostic) {
    recordSessionPersistenceInvalidation(diagnostic);
  }
}

function persistenceScopeFromTimelineIdentity(
  identity: TimelineCheckpointStreamIdentity,
): SessionPersistenceIdentityScope {
  return {
    backendScopeID: identity.backendScopeID,
    factorySessionID: identity.factorySessionID,
    streamGenerationID: identity.streamGenerationID,
  };
}

function checkpointStorageKey(
  identity: TimelineCheckpointStreamIdentity | null,
): string | null {
  const normalizedIdentity = normalizeStreamDerivedCacheIdentity(identity);
  if (!normalizedIdentity) {
    return null;
  }
  return streamDerivedCheckpointStorageKey(normalizedIdentity);
}

function matchesStreamIdentity(
  actual: TimelineCheckpointStreamIdentity | null | undefined,
  expected: TimelineCheckpointStreamIdentity | null,
): boolean {
  const normalizedActual = normalizeStreamDerivedCacheIdentity(actual);
  const normalizedExpected = normalizeStreamDerivedCacheIdentity(expected);
  if (normalizedActual == null || normalizedExpected == null) {
    return false;
  }
  return (
    normalizedActual.backendScopeID === normalizedExpected.backendScopeID &&
    normalizedActual.factorySessionID === normalizedExpected.factorySessionID &&
    normalizedActual.logicalSessionKeyID ===
      normalizedExpected.logicalSessionKeyID &&
    normalizedActual.streamGenerationID === normalizedExpected.streamGenerationID
  );
}

export async function findStoredCheckpointEnvelopeByFactorySessionID(
  indexedDB: IndexedDBLike | undefined,
  factorySessionID: string,
): Promise<TimelineCheckpointEnvelope | null> {
  const normalizedFactorySessionID = factorySessionID.trim();
  if (!indexedDB || normalizedFactorySessionID === "") {
    return null;
  }

  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const envelopes = await requestToPromise<TimelineCheckpointEnvelope[]>(
      store.getAll(),
    );
    return (
      envelopes.find(
        (envelope) =>
          envelope.schemaVersion === CHECKPOINT_SCHEMA_VERSION_GUARDED &&
          matchesStoredCheckpointFactorySessionID(
            envelope,
            normalizedFactorySessionID,
          ),
      ) ?? null
    );
  } catch {
    return null;
  } finally {
    database.close();
  }
}

export async function clearStoredTimelineCheckpointsForFactorySessionID(
  indexedDB: IndexedDBLike | undefined,
  factorySessionID: string,
): Promise<void> {
  const envelope = await findStoredCheckpointEnvelopeByFactorySessionID(
    indexedDB,
    factorySessionID,
  );
  if (!envelope) {
    return;
  }
  await clearTimelineCheckpoint(indexedDB, envelope.streamIdentity);
}

export async function persistTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  checkpoint: FactoryTimelineCheckpoint | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  const normalizedStreamIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !checkpoint || !storageKey || !normalizedStreamIdentity) {
    return;
  }

  const envelope = {
    checkpoint: buildPersistedCheckpoint(checkpoint),
    schemaVersion: CHECKPOINT_SCHEMA_VERSION_GUARDED,
    storageKey,
    streamIdentity: normalizedStreamIdentity,
  } satisfies TimelineCheckpointEnvelope;

  try {
    await writeIndexedCheckpoint(indexedDB, envelope);
  } catch {
    await deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {});
  }
}

export async function purgeLegacyTimelineCheckpoints(
  indexedDB: IndexedDBLike | undefined,
): Promise<void> {
  if (!indexedDB) {
    return;
  }
  await deleteIndexedCheckpoint(indexedDB, DEFAULT_FACTORY_SESSION_ID).catch(
    () => {},
  );
}

export async function readTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<FactoryTimelineCheckpoint | null> {
  const normalizedStreamIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !storageKey) {
    return null;
  }

  try {
    const envelope = await readIndexedCheckpoint(indexedDB, storageKey);
    if (envelope) {
      const checkpoint = parseStoredCheckpoint(
        envelope,
        normalizedStreamIdentity,
      );
      if (!checkpoint) {
        await deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {});
      }
      return checkpoint;
    }
  } catch {
    await deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {});
  }

  return null;
}

export async function deleteTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  await clearTimelineCheckpoint(indexedDB, streamIdentity);
}
