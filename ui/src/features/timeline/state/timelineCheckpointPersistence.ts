import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../api/session-routing";
import {
  identityMismatchDiagnostic,
  recordSessionPersistenceInvalidation,
  type SessionPersistenceIdentityScope,
  userClearedSessionsDiagnostic,
} from "../../dashboard/public/session-persistence-diagnostics";
import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
  streamDerivedCheckpointStorageKey,
} from "../lib/stream-derived-cache-identity";
import {
  type IndexedDBLike,
  indexedDBRequestToPromise,
  openCheckpointDatabase,
} from "./checkpoint-persistence/indexedDBCheckpointRequests";
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
  const normalizedFactorySessionID = factorySessionID?.trim() ?? "";
  if (isDefaultFactorySessionID(normalizedFactorySessionID)) {
    return null;
  }
  return normalizedFactorySessionID;
}

function matchesStoredCheckpointFactorySessionID(
  envelope: TimelineCheckpointEnvelope,
  factorySessionID: string,
): boolean {
  const requestedSessionID = factorySessionID.trim();
  const storedFactorySessionID =
    envelope.streamIdentity?.factorySessionID?.trim() ?? "";
  if (requestedSessionID === "") {
    return false;
  }
  if (storedFactorySessionID !== "") {
    return storedFactorySessionID === requestedSessionID;
  }
  return envelope.sessionID?.trim() === requestedSessionID;
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
    await indexedDBRequestToPromise(store.put(envelope));
  } finally {
    database.close();
  }
}

async function readIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  storageKey: string,
  signal?: AbortSignal,
): Promise<TimelineCheckpointEnvelope | null> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const result = await indexedDBRequestToPromise<
      TimelineCheckpointEnvelope | undefined
    >(store.get(storageKey), transaction, signal);
    return result ?? null;
  } finally {
    database.close();
  }
}

async function deleteIndexedCheckpoint(
  indexedDB: IndexedDBLike,
  storageKey: string,
  signal?: AbortSignal,
): Promise<void> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    await indexedDBRequestToPromise(
      store.delete(storageKey),
      transaction,
      signal,
    );
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
    if (
      envelope.schemaVersion !== CHECKPOINT_SCHEMA_VERSION_GUARDED ||
      !isSupportedPersistedTimelineCheckpoint(envelope.checkpoint)
    ) {
      const storageKey = envelope.storageKey?.trim() ?? "";
      if (storageKey !== "") {
        await deleteIndexedCheckpoint(
          indexedDB,
          storageKey,
          options.signal,
        ).catch(() => {});
      }
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
  options: { signal?: AbortSignal } = {},
): Promise<void> {
  const normalizedSessionID = normalizeConcreteFactorySessionID(sessionID);
  if (!indexedDB || !normalizedSessionID) {
    return;
  }

  try {
    const envelopes = await listIndexedCheckpoints(indexedDB, options.signal);
    if (options.signal?.aborted) {
      return;
    }
    const storageKeys = envelopes
      .filter((envelope) =>
        matchesStoredCheckpointFactorySessionID(envelope, normalizedSessionID),
      )
      .map((envelope) => envelope.storageKey)
      .filter((storageKey) => storageKey.trim() !== "");

    await Promise.all(
      storageKeys.map((storageKey) =>
        deleteIndexedCheckpoint(indexedDB, storageKey, options.signal).catch(
          () => {},
        ),
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
  return identity;
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
    normalizedActual.streamGenerationID ===
      normalizedExpected.streamGenerationID
  );
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
    // Preserve any previously committed checkpoint when its replacement fails.
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
  options: { signal?: AbortSignal } = {},
): Promise<FactoryTimelineCheckpoint | null> {
  const normalizedStreamIdentity =
    normalizeStreamDerivedCacheIdentity(streamIdentity);
  const storageKey = checkpointStorageKey(normalizedStreamIdentity);
  if (!indexedDB || !storageKey) {
    return null;
  }

  try {
    const envelope = await readIndexedCheckpoint(
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
        await deleteIndexedCheckpoint(
          indexedDB,
          storageKey,
          options.signal,
        ).catch(() => {});
      }
      return checkpoint;
    }
  } catch {
    if (!options.signal?.aborted) {
      await deleteIndexedCheckpoint(
        indexedDB,
        storageKey,
        options.signal,
      ).catch(() => {});
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
