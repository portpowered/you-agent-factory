export interface IndexedDBLike {
  open: IDBFactory["open"];
}

const CHECKPOINT_DB_NAME = "agentFactoryTimelineCheckpoints";
const CHECKPOINT_DB_VERSION = 3;
const CHECKPOINT_STORE_NAME = "checkpoints";

export function openCheckpointDatabase(
  indexedDB: IndexedDBLike,
): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(CHECKPOINT_DB_NAME, CHECKPOINT_DB_VERSION);
    request.onupgradeneeded = () => {
      const database = request.result;
      if (database.objectStoreNames.contains(CHECKPOINT_STORE_NAME)) {
        database.deleteObjectStore?.(CHECKPOINT_STORE_NAME);
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

export function indexedDBRequestToPromise<T>(
  request: IDBRequest<T>,
  transaction?: IDBTransaction,
  signal?: AbortSignal,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const finish = (callback: () => void) => {
      signal?.removeEventListener("abort", onAbort);
      callback();
    };
    const onAbort = () => {
      try {
        transaction?.abort();
      } catch {
        // The transaction may already be complete.
      }
      finish(() =>
        reject(
          new DOMException(
            "timeline checkpoint operation aborted",
            "AbortError",
          ),
        ),
      );
    };
    request.onsuccess = () => finish(() => resolve(request.result));
    request.onerror = () => finish(() => reject(request.error));
    if (signal?.aborted) {
      onAbort();
      return;
    }
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}
