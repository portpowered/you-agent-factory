export interface IndexedDBLike {
  open: IDBFactory["open"];
}

const CHECKPOINT_DB_NAME = "agentFactoryTimelineCheckpoints";
const CHECKPOINT_DB_VERSION = 3;
const CHECKPOINT_STORE_NAME = "checkpoints";

export async function readCheckpointDatabaseRecord<T>(
  indexedDB: IndexedDBLike,
  storageKey: string,
  signal?: AbortSignal,
): Promise<T | null> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(CHECKPOINT_STORE_NAME, "readonly");
    const result = await indexedDBRequestToPromise<T | undefined>(
      transaction.objectStore(CHECKPOINT_STORE_NAME).get(storageKey),
      transaction,
      signal,
    );
    return result ?? null;
  } finally {
    database.close();
  }
}

export async function deleteCheckpointDatabaseRecord(
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
    const requestOutcome = indexedDBRequestToPromise(
      transaction.objectStore(CHECKPOINT_STORE_NAME).delete(storageKey),
      transaction,
      signal,
    ).then(
      () => ({ succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const transactionOutcome = indexedDBTransactionToPromise(transaction).then(
      () => ({ succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const [request, durableTransaction] = await Promise.all([
      requestOutcome,
      transactionOutcome,
    ]);
    if (!request.succeeded) throw request.error;
    if (!durableTransaction.succeeded) throw durableTransaction.error;
  } finally {
    database.close();
  }
}

export async function replaceCheckpointDatabaseRecord<T>(
  indexedDB: IndexedDBLike,
  storageKey: string,
  value: T,
  shouldReplace: (stored: T | undefined) => boolean,
): Promise<boolean> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const requestOutcome = new Promise<boolean>((resolve, reject) => {
      const readRequest = store.get(storageKey) as IDBRequest<T | undefined>;
      readRequest.onerror = () => reject(readRequest.error);
      readRequest.onsuccess = () => {
        try {
          if (!shouldReplace(readRequest.result)) {
            resolve(false);
            return;
          }
          const writeRequest = store.put(value);
          writeRequest.onerror = () => reject(writeRequest.error);
          writeRequest.onsuccess = () => resolve(true);
        } catch (error) {
          reject(error);
        }
      };
    }).then(
      (replaced) => ({ replaced, succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const transactionOutcome = indexedDBTransactionToPromise(transaction).then(
      () => ({ succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const request = await requestOutcome;
    // A rejected candidate made no mutation. The authoritative read is enough
    // to settle it; successful and failed replacements still await durability.
    if (request.succeeded && !request.replaced) {
      return false;
    }
    const durableTransaction = await transactionOutcome;
    if (!request.succeeded) throw request.error;
    if (!durableTransaction.succeeded) throw durableTransaction.error;
    return request.replaced;
  } finally {
    database.close();
  }
}

export async function deleteCheckpointDatabaseRecordsMatching<
  T extends { storageKey: string },
>(
  indexedDB: IndexedDBLike,
  matches: (stored: T) => boolean,
  signal?: AbortSignal,
): Promise<void> {
  const database = await openCheckpointDatabase(indexedDB);
  try {
    const transaction = database.transaction(
      CHECKPOINT_STORE_NAME,
      "readwrite",
    );
    const store = transaction.objectStore(CHECKPOINT_STORE_NAME);
    const requestOutcome = new Promise<boolean>((resolve, reject) => {
      const finish = (callback: () => void) => {
        signal?.removeEventListener("abort", onAbort);
        callback();
      };
      const onAbort = () => {
        try {
          transaction.abort();
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
      const readRequest = store.getAll() as IDBRequest<T[]>;
      readRequest.onerror = () => finish(() => reject(readRequest.error));
      readRequest.onsuccess = () => {
        const matchingRecords = readRequest.result.filter(matches);
        if (matchingRecords.length === 0) {
          finish(() => resolve(false));
          return;
        }
        let remaining = matchingRecords.length;
        for (const record of matchingRecords) {
          const deleteRequest = store.delete(record.storageKey);
          deleteRequest.onerror = () =>
            finish(() => reject(deleteRequest.error));
          deleteRequest.onsuccess = () => {
            remaining -= 1;
            if (remaining === 0) finish(() => resolve(true));
          };
        }
      };
      if (signal?.aborted) {
        onAbort();
        return;
      }
      signal?.addEventListener("abort", onAbort, { once: true });
    }).then(
      (deleted) => ({ deleted, succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const transactionOutcome = indexedDBTransactionToPromise(transaction).then(
      () => ({ succeeded: true }) as const,
      (error: unknown) => ({ error, succeeded: false }) as const,
    );
    const request = await requestOutcome;
    if (request.succeeded && !request.deleted) return;
    const durableTransaction = await transactionOutcome;
    if (!request.succeeded) throw request.error;
    if (!durableTransaction.succeeded) throw durableTransaction.error;
  } finally {
    database.close();
  }
}

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

export function indexedDBTransactionToPromise(
  transaction: IDBTransaction,
): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onabort = () =>
      reject(
        transaction.error ??
          new Error("timeline checkpoint IndexedDB transaction aborted"),
      );
    transaction.onerror = () =>
      reject(
        transaction.error ??
          new Error("timeline checkpoint IndexedDB transaction failed"),
      );
  });
}
