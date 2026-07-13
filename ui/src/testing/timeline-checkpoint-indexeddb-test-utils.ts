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
    transaction: () => {
      const transaction = {
        onabort: null,
        oncomplete: null,
        onerror: null,
        objectStore: () => ({
          delete: (key: string) =>
            indexedDBTestRequest(
              undefined,
              () => {
                records.delete(key);
              },
              () =>
                (transaction.oncomplete as ((event: Event) => void) | null)?.(
                  {} as Event,
                ),
            ),
          get: (key: string) => indexedDBTestRequest(records.get(key)),
          getAll: () => indexedDBTestRequest([...records.values()]),
          put: (value: StoredCheckpointEnvelope) =>
            indexedDBTestRequest(
              value.storageKey ?? "",
              () => {
                if (value.storageKey) {
                  records.set(value.storageKey, value);
                }
              },
              () =>
                (transaction.oncomplete as ((event: Event) => void) | null)?.(
                  {} as Event,
                ),
            ),
        }),
      };
      return transaction;
    },
  };

  return {
    indexedDB: {
      open: () => {
        const request = indexedDBTestRequest(database);
        queueMicrotask(() =>
          request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        );
        return request;
      },
    } as unknown as IDBFactory,
    records,
  };
}

export function indexedDBTestRequest<T>(
  result: T,
  beforeSuccess?: () => void,
  afterSuccess?: () => void,
) {
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
    afterSuccess?.();
  });

  return request;
}

export function indexedDBErrorTestRequest<T>(
  error: Error,
  afterError?: () => void,
): IDBRequest<T> {
  const request = {
    error,
    onerror: null,
    onsuccess: null,
    result: undefined,
  } as unknown as IDBRequest<T>;
  queueMicrotask(() => {
    request.onerror?.({} as Event);
    afterError?.();
  });
  return request;
}
