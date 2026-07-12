import {
  type IndexedDBLike,
  indexedDBRequestToPromise,
  openCheckpointDatabase,
} from "./indexedDBCheckpointRequests";

const CHECKPOINT_STORE_NAME = "checkpoints";

export async function deletePersistedTimelineCheckpoint(
  indexedDB: IndexedDBLike | undefined,
  persistedCheckpoint: { storageKey: string } | null,
): Promise<void> {
  const storageKey = persistedCheckpoint?.storageKey.trim() ?? "";
  if (!indexedDB || !storageKey) {
    return;
  }

  try {
    const database = await openCheckpointDatabase(indexedDB);
    try {
      const transaction = database.transaction(
        CHECKPOINT_STORE_NAME,
        "readwrite",
      );
      await indexedDBRequestToPromise(
        transaction.objectStore(CHECKPOINT_STORE_NAME).delete(storageKey),
      );
    } finally {
      database.close();
    }
  } catch {
    // Best-effort invalidation must not block fresh stream hydration.
  }
}
