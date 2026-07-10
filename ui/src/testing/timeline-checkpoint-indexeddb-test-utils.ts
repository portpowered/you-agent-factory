interface StoredCheckpointEnvelope {
  storageKey?: string;
}

export function createTimelineCheckpointIndexedDBTestDouble(): {
  indexedDB: IDBFactory;
  records: Map<string, StoredCheckpointEnvelope>;
} {
  const records = new Map<string, StoredCheckpointEnvelope>();
  const database = {
    close: () => {},
    createObjectStore: () => undefined,
    deleteObjectStore: () => undefined,
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => {
            records.delete(key);
          }),
        get: (key: string) => indexedDBRequest(records.get(key)),
        getAll: () => indexedDBRequest([...records.values()]),
        put: (value: StoredCheckpointEnvelope) =>
          indexedDBRequest(value.storageKey ?? "", () => {
            if (value.storageKey) {
              records.set(value.storageKey, value);
            }
          }),
      }),
    }),
  };

  return {
    indexedDB: {
      open: () => {
        const request = indexedDBRequest(database);
        queueMicrotask(() =>
          request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        );
        return request;
      },
    } as unknown as IDBFactory,
    records,
  };
}

function indexedDBRequest<T>(result: T, beforeSuccess?: () => void) {
  const request = {
    error: null,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result,
  } as unknown as IDBRequest<T> & {
    onblocked?: ((event: Event) => void) | null;
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  queueMicrotask(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  });

  return request;
}
