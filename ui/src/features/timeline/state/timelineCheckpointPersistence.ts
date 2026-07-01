import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  identityMismatchDiagnostic,
  recordSessionPersistenceInvalidation,
  userClearedSessionsDiagnostic,
  type SessionPersistenceIdentityScope,
} from "../../dashboard/public";
import type { FactoryTimelineCheckpoint } from "./timeline/storeState";
import type { ReplayWorldState } from "./timeline/types";

const CHECKPOINT_SCHEMA_VERSION_GUARDED = 2;
const CHECKPOINT_DB_NAME = "agentFactoryTimelineCheckpoints";
const CHECKPOINT_DB_VERSION = 2;
const CHECKPOINT_STORE_NAME = "checkpoints";
const MAX_COMPACT_TEXT_CHARS = 512;

interface TimelineCheckpointEnvelope {
  checkpoint: PersistedTimelineCheckpoint;
  schemaVersion: number;
  sessionID: string;
  storageKey: string;
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
  sessionID: string | null,
  expectedIdentity: TimelineCheckpointStreamIdentity | null,
): FactoryTimelineCheckpoint | null {
  if (
    envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
    envelope.sessionID !== sessionID ||
    !envelope.checkpoint?.replayState
  ) {
    return null;
  }
  if (!matchesStreamIdentity(envelope.streamIdentity, expectedIdentity)) {
    recordCheckpointIdentityMismatch(
      envelope.streamIdentity,
      expectedIdentity,
      sessionID,
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

function checkpointStorageKey(
  identity: TimelineCheckpointStreamIdentity | null,
): string | null {
  const normalizedIdentity = normalizeStreamIdentity(identity);
  if (!normalizedIdentity) {
    return null;
  }
  return [
    normalizedIdentity.backendScopeID,
    normalizedIdentity.factorySessionID,
    normalizedIdentity.streamGenerationID,
  ].join("::");
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
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !sessionID || !checkpoint || !storageKey) {
    return;
  }

  const envelope = {
    checkpoint: buildPersistedCheckpoint(checkpoint),
    schemaVersion: CHECKPOINT_SCHEMA_VERSION_GUARDED,
    sessionID,
    storageKey,
    ...(normalizedStreamIdentity
      ? { streamIdentity: normalizedStreamIdentity }
      : {}),
  } satisfies TimelineCheckpointEnvelope;

  try {
    await writeIndexedCheckpoint(indexedDB, envelope);
  } catch {
    await deleteIndexedCheckpoint(indexedDB, storageKey).catch(() => {});
  }
}

export async function readTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<FactoryTimelineCheckpoint | null> {
  const normalizedStreamIdentity = normalizeStreamIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !sessionID || !storageKey) {
    return null;
  }

  try {
    const envelope = await readIndexedCheckpoint(indexedDB, storageKey);
    if (envelope) {
      const checkpoint = parseStoredCheckpoint(
        envelope,
        sessionID,
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
  sessionID: string | null,
): Promise<void> {
  if (!indexedDB || !sessionID) {
    return;
  }

  await deleteIndexedCheckpoint(indexedDB, sessionID).catch(() => {});
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
