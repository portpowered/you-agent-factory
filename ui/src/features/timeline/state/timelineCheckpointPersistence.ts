import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  identityMismatchDiagnostic,
  recordSessionPersistenceInvalidation,
  type SessionPersistenceIdentityScope,
  userClearedSessionsDiagnostic,
} from "../../dashboard/public/session-persistence-diagnostics";
import {
  type StreamDerivedCacheIdentity,
  streamDerivedCheckpointStorageKey,
} from "../lib/stream-derived-cache-identity";
import {
  checkpointSyncIdentityMatchesStreamIdentity,
  normalizeFactorySessionUUID,
  normalizeStoredTimelineCheckpointIdentity,
  normalizeTimelineCheckpointIdentity,
  timelineCheckpointIdentitiesMatch,
} from "./checkpoint-persistence/identity/timelineCheckpointIdentity";
import {
  deleteCheckpointDatabaseRecord,
  deleteCheckpointDatabaseRecordsMatching,
  type IndexedDBLike,
  indexedDBRequestToPromise,
  openCheckpointDatabase,
  readCheckpointDatabaseRecord,
  replaceCheckpointDatabaseRecord,
} from "./checkpoint-persistence/indexedDBCheckpointRequests";
import {
  enqueueOrderedCheckpointClear,
  enqueueOrderedCheckpointWrite,
} from "./checkpoint-persistence/ordering/orderedCheckpointWriter";
import {
  buildPersistedCheckpoint,
  CHECKPOINT_SCHEMA_VERSION_GUARDED,
  hydrateCheckpoint,
  isSupportedPersistedTimelineCheckpoint,
  type PersistedTimelineCheckpoint,
} from "./checkpoint-persistence/timelineCheckpointCodec";
import type { FactoryTimelineCheckpoint } from "./timeline/storeState";

const CHECKPOINT_STORE_NAME = "checkpoints";

export type TimelineCheckpointStreamIdentity = StreamDerivedCacheIdentity;

interface TimelineCheckpointEnvelope {
  checkpoint: PersistedTimelineCheckpoint;
  schemaVersion: number;
  sessionID?: string;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity;
}

function normalizeConcreteFactorySessionID(
  factorySessionID: string | null | undefined,
): string | null {
  return normalizeFactorySessionUUID(factorySessionID);
}

function matchesStoredCheckpointFactorySessionID(
  envelope: TimelineCheckpointEnvelope,
  factorySessionID: string,
): boolean {
  const requestedSessionID = normalizeFactorySessionUUID(factorySessionID);
  const legacySessionID = normalizeFactorySessionUUID(envelope.sessionID);
  const storedFactorySessionID = normalizeFactorySessionUUID(
    envelope.streamIdentity?.factorySessionID,
  );
  if (!requestedSessionID) {
    return false;
  }
  if (legacySessionID) {
    return legacySessionID === requestedSessionID;
  }
  if (storedFactorySessionID) {
    return storedFactorySessionID === requestedSessionID;
  }
  return false;
}

export interface PersistedTimelineCheckpointPeek {
  checkpoint: FactoryTimelineCheckpoint;
  storageKey: string;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

function normalizedEnvelopeIdentity(
  envelope: TimelineCheckpointEnvelope,
): TimelineCheckpointStreamIdentity | null {
  return normalizeStoredTimelineCheckpointIdentity(
    envelope.streamIdentity,
    envelope.storageKey,
    envelope.checkpoint.syncIdentity,
  );
}

async function deleteRejectedEnvelope(
  indexedDB: IndexedDBLike,
  envelope: TimelineCheckpointEnvelope,
  signal?: AbortSignal,
): Promise<void> {
  const storageKey = envelope.storageKey?.trim() ?? "";
  if (storageKey === "") {
    return;
  }
  const streamIdentity = normalizeTimelineCheckpointIdentity(
    envelope.streamIdentity,
  );
  if (streamIdentity && checkpointStorageKey(streamIdentity) === storageKey) {
    await clearTimelineCheckpoint(indexedDB, streamIdentity, { signal });
    return;
  }
  await deleteCheckpointDatabaseRecord(indexedDB, storageKey, signal).catch(
    () => {},
  );
}

async function listIndexedCheckpoints(
  indexedDB: IndexedDBLike,
  signal?: AbortSignal,
): Promise<TimelineCheckpointEnvelope[]> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const result = await indexedDBRequestToPromise<
      TimelineCheckpointEnvelope[]
    >(store.getAll(), transaction, signal);
    return result ?? [];
  } finally {
    database.close();
  }
}

export async function peekPersistedTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
  options: { signal?: AbortSignal } = {},
): Promise<PersistedTimelineCheckpointPeek | null> {
  const normalizedSessionID = normalizeConcreteFactorySessionID(sessionID);
  if (!indexedDB || !normalizedSessionID) {
    return null;
  }

  try {
    const envelopes = await listIndexedCheckpoints(indexedDB, options.signal);
    if (options.signal?.aborted) {
      return null;
    }
    const envelope = envelopes.find((candidate) =>
      matchesStoredCheckpointFactorySessionID(candidate, normalizedSessionID),
    );
    if (!envelope) {
      return null;
    }
    const checkpointSupported = isSupportedPersistedTimelineCheckpoint(
      envelope.checkpoint,
    );
    const streamIdentity = checkpointSupported
      ? normalizedEnvelopeIdentity(envelope)
      : null;
    if (
      envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
      !checkpointSupported ||
      !streamIdentity
    ) {
      await deleteRejectedEnvelope(indexedDB, envelope, options.signal);
      return null;
    }

    return {
      checkpoint: hydrateCheckpoint(envelope.checkpoint),
      storageKey: envelope.storageKey,
      streamIdentity,
    };
  } catch {
    return null;
  }
}

export async function clearTimelineCheckpointsForSession(
  indexedDB: IndexedDBLike | undefined,
  sessionID: string | null,
  options: { signal?: AbortSignal } = {},
): Promise<void> {
  const normalizedSessionID = normalizeConcreteFactorySessionID(sessionID);
  if (!indexedDB || !normalizedSessionID) {
    return;
  }

  try {
    await deleteCheckpointDatabaseRecordsMatching<TimelineCheckpointEnvelope>(
      indexedDB,
      (envelope) =>
        matchesStoredCheckpointFactorySessionID(
          envelope,
          normalizedSessionID,
        ),
      options.signal,
    );
  } catch {
    // Best-effort cleanup for stale reconnect state.
  }
}

export async function clearTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
  options: {
    requestedSessionID?: string;
    signal?: AbortSignal;
    userInitiated?: boolean;
  } = {},
): Promise<void> {
  const normalizedStreamIdentity =
    normalizeTimelineCheckpointIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !storageKey || !normalizedStreamIdentity) {
    return;
  }
  if (options.userInitiated) {
    const requestedSessionID = normalizeConcreteFactorySessionID(
      options.requestedSessionID,
    );
    if (
      !requestedSessionID ||
      requestedSessionID !== normalizedStreamIdentity.factorySessionID
    ) {
      return;
    }
    recordSessionPersistenceInvalidation(
      userClearedSessionsDiagnostic(
        persistenceScopeFromTimelineIdentity(normalizedStreamIdentity),
        requestedSessionID,
      ),
    );
  }
  await enqueueOrderedCheckpointClear(indexedDB, normalizedStreamIdentity, () =>
    deleteCheckpointDatabaseRecord(indexedDB, storageKey, options.signal),
  ).catch(() => {});
}

function parseStoredCheckpoint(
  envelope: TimelineCheckpointEnvelope,
  expectedIdentity: TimelineCheckpointStreamIdentity | null,
): FactoryTimelineCheckpoint | null {
  if (
    envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
    !isSupportedPersistedTimelineCheckpoint(envelope.checkpoint)
  ) {
    return null;
  }
  if (
    !timelineCheckpointIdentitiesMatch(
      envelope.streamIdentity,
      expectedIdentity,
    )
  ) {
    recordCheckpointIdentityMismatch(
      envelope.streamIdentity,
      expectedIdentity,
      envelope.streamIdentity?.factorySessionID ?? null,
    );
    return null;
  }
  if (!normalizedEnvelopeIdentity(envelope)) {
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
  return identity;
}

function checkpointStorageKey(
  identity: TimelineCheckpointStreamIdentity | null,
): string | null {
  const normalizedIdentity = normalizeTimelineCheckpointIdentity(identity);
  if (!normalizedIdentity) {
    return null;
  }
  return streamDerivedCheckpointStorageKey(normalizedIdentity);
}

export async function findStoredCheckpointEnvelopeByFactorySessionID(
  indexedDB: IndexedDBLike | undefined,
  factorySessionID: string,
): Promise<TimelineCheckpointEnvelope | null> {
  const normalizedFactorySessionID =
    normalizeConcreteFactorySessionID(factorySessionID);
  if (!indexedDB || !normalizedFactorySessionID) {
    return null;
  }

  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const envelopes = await indexedDBRequestToPromise<
      TimelineCheckpointEnvelope[]
    >(store.getAll());
    return (
      envelopes.find(
        (envelope) =>
          envelope.schemaVersion === CHECKPOINT_SCHEMA_VERSION_GUARDED &&
          isSupportedPersistedTimelineCheckpoint(envelope.checkpoint) &&
          normalizedEnvelopeIdentity(envelope) != null &&
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
    normalizeTimelineCheckpointIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  const persistedCheckpoint =
    checkpoint && buildPersistedCheckpoint(checkpoint);
  if (checkpoint && !persistedCheckpoint)
    return clearTimelineCheckpoint(indexedDB, normalizedStreamIdentity);
  if (
    !indexedDB ||
    !persistedCheckpoint ||
    !storageKey ||
    !normalizedStreamIdentity ||
    !checkpointSyncIdentityMatchesStreamIdentity(
      persistedCheckpoint.syncIdentity,
      normalizedStreamIdentity,
    )
  ) {
    return;
  }

  const envelope = {
    checkpoint: persistedCheckpoint,
    schemaVersion: CHECKPOINT_SCHEMA_VERSION_GUARDED,
    storageKey,
    streamIdentity: normalizedStreamIdentity,
  } satisfies TimelineCheckpointEnvelope;

  try {
    await enqueueOrderedCheckpointWrite(
      indexedDB,
      normalizedStreamIdentity,
      persistedCheckpoint.afterSequence,
      () =>
        replaceCheckpointDatabaseRecord<TimelineCheckpointEnvelope>(
          indexedDB,
          storageKey,
          envelope,
          (committedEnvelope) => {
            if (!committedEnvelope) {
              return true;
            }
            const committedCheckpoint = parseStoredCheckpoint(
              committedEnvelope,
              normalizedStreamIdentity,
            );
            if (!committedCheckpoint) {
              return true;
            }
            return (
              persistedCheckpoint.afterSequence !== undefined &&
              (committedCheckpoint.afterSequence === undefined ||
                persistedCheckpoint.afterSequence >
                  committedCheckpoint.afterSequence)
            );
          },
        ),
    );
  } catch {
    // Preserve any previously committed checkpoint when its replacement fails.
  }
}

export async function purgeLegacyTimelineCheckpoints(
  indexedDB: IndexedDBLike | undefined,
): Promise<void> {
  if (!indexedDB) return;
  await deleteCheckpointDatabaseRecord(
    indexedDB,
    DEFAULT_FACTORY_SESSION_ID,
  ).catch(() => {});
}

export async function readTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
  options: { signal?: AbortSignal } = {},
): Promise<FactoryTimelineCheckpoint | null> {
  const normalizedStreamIdentity =
    normalizeTimelineCheckpointIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !storageKey) {
    return null;
  }

  try {
    const envelope =
      await readCheckpointDatabaseRecord<TimelineCheckpointEnvelope>(
        indexedDB,
        storageKey,
        options.signal,
      );
    if (options.signal?.aborted) {
      return null;
    }
    if (envelope) {
      const checkpoint = parseStoredCheckpoint(
        envelope,
        normalizedStreamIdentity,
      );
      if (!checkpoint) {
        await clearTimelineCheckpoint(indexedDB, normalizedStreamIdentity);
      }
      return checkpoint;
    }
  } catch {
    if (!options.signal?.aborted) {
      await clearTimelineCheckpoint(indexedDB, normalizedStreamIdentity);
    }
  }

  return null;
}

export async function deleteTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  streamIdentity: TimelineCheckpointStreamIdentity | null,
): Promise<void> {
  await clearTimelineCheckpoint(indexedDB, streamIdentity);
}
