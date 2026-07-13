import {
  normalizeStreamDerivedCacheIdentity,
  type StreamDerivedCacheIdentity,
  streamDerivedCheckpointStorageKey,
} from "../../../lib/stream-derived-cache-identity";

const FACTORY_SESSION_UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const NIL_FACTORY_SESSION_UUID = "00000000-0000-0000-0000-000000000000";

/**
 * Durable checkpoints are only safe once the backend has resolved an exact
 * Factory Session UUID and complete stream identity. Other stream-derived
 * caches may exist briefly while session routing is still resolving.
 */
export function normalizeTimelineCheckpointIdentity(
  identity: Partial<StreamDerivedCacheIdentity> | null | undefined,
): StreamDerivedCacheIdentity | null {
  const normalized = normalizeStreamDerivedCacheIdentity(identity);
  const factorySessionID = normalizeFactorySessionUUID(
    normalized?.factorySessionID,
  );
  if (!normalized || !factorySessionID) {
    return null;
  }
  return { ...normalized, factorySessionID };
}

export function normalizeFactorySessionUUID(
  factorySessionID: string | null | undefined,
): string | null {
  const normalized = factorySessionID?.trim() ?? "";
  if (!FACTORY_SESSION_UUID_PATTERN.test(normalized)) {
    return null;
  }
  const canonical = normalized.toLowerCase();
  return canonical === NIL_FACTORY_SESSION_UUID ? null : canonical;
}

export function timelineCheckpointIdentitiesMatch(
  actual: Partial<StreamDerivedCacheIdentity> | null | undefined,
  expected: Partial<StreamDerivedCacheIdentity> | null | undefined,
): boolean {
  const normalizedActual = normalizeTimelineCheckpointIdentity(actual);
  const normalizedExpected = normalizeTimelineCheckpointIdentity(expected);
  return (
    normalizedActual != null &&
    normalizedExpected != null &&
    normalizedActual.backendScopeID === normalizedExpected.backendScopeID &&
    normalizedActual.factorySessionID === normalizedExpected.factorySessionID &&
    normalizedActual.logicalSessionKeyID ===
      normalizedExpected.logicalSessionKeyID &&
    normalizedActual.streamGenerationID ===
      normalizedExpected.streamGenerationID
  );
}

interface CheckpointSyncIdentity {
  backendScopeId: string;
  factorySessionId: string;
  logicalSessionKeyId: string;
  streamGenerationId: string;
}

export function checkpointSyncIdentityMatchesStreamIdentity(
  syncIdentity: CheckpointSyncIdentity | null | undefined,
  streamIdentity: StreamDerivedCacheIdentity,
): boolean {
  if (!syncIdentity) {
    return true;
  }
  return timelineCheckpointIdentitiesMatch(
    {
      backendScopeID: syncIdentity.backendScopeId,
      factorySessionID: syncIdentity.factorySessionId,
      logicalSessionKeyID: syncIdentity.logicalSessionKeyId,
      streamGenerationID: syncIdentity.streamGenerationId,
    },
    streamIdentity,
  );
}

export function normalizeStoredTimelineCheckpointIdentity(
  identity: Partial<StreamDerivedCacheIdentity> | null | undefined,
  storageKey: string | null | undefined,
  syncIdentity?: CheckpointSyncIdentity | null,
): StreamDerivedCacheIdentity | null {
  const normalized = normalizeTimelineCheckpointIdentity(identity);
  if (
    !normalized ||
    storageKey?.trim() !== streamDerivedCheckpointStorageKey(normalized) ||
    !checkpointSyncIdentityMatchesStreamIdentity(syncIdentity, normalized)
  ) {
    return null;
  }
  return normalized;
}
