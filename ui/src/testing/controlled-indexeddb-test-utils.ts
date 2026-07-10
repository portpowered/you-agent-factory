export type ControlledIndexedDBOperation =
  | "delete"
  | "get"
  | "getAll"
  | "open"
  | "put";

interface StoredRecord {
  storageKey?: string;
}

interface ControlledRequest<T> {
  beforeSuccess?: () => void;
  operation: ControlledIndexedDBOperation;
  request: IDBRequest<T>;
  result: () => T;
}

export function createControlledIndexedDBTestDouble<
  RecordType extends StoredRecord,
>() {
  const records = new Map<string, RecordType>();
  const pending: ControlledRequest<unknown>[] = [];

  function controlledRequest<T>(
    operation: ControlledIndexedDBOperation,
    result: () => T,
    beforeSuccess?: () => void,
  ): IDBRequest<T> {
    const request = {
      error: null,
      onblocked: null,
      onerror: null,
      onsuccess: null,
      onupgradeneeded: null,
      result: undefined,
    } as unknown as IDBRequest<T>;
    pending.push({
      beforeSuccess,
      operation,
      request,
      result,
    } as ControlledRequest<unknown>);
    return request;
  }

  const database = {
    close: () => {},
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          controlledRequest(
            "delete",
            () => undefined,
            () => records.delete(key),
          ),
        get: (key: string) => controlledRequest("get", () => records.get(key)),
        getAll: () => controlledRequest("getAll", () => [...records.values()]),
        put: (value: RecordType) =>
          controlledRequest(
            "put",
            () => value.storageKey ?? "",
            () => {
              if (value.storageKey) {
                records.set(value.storageKey, value);
              }
            },
          ),
      }),
    }),
  };

  function take(
    operation: ControlledIndexedDBOperation,
  ): ControlledRequest<unknown> {
    const index = pending.findIndex(
      (request) => request.operation === operation,
    );
    const controlled = pending[index];
    if (index < 0 || !controlled) {
      throw new Error(`no pending IndexedDB ${operation} request`);
    }
    pending.splice(index, 1);
    return controlled;
  }

  function succeed(operation: ControlledIndexedDBOperation): void {
    const controlled = take(operation);
    controlled.beforeSuccess?.();
    Object.defineProperty(controlled.request, "result", {
      configurable: true,
      value: controlled.result(),
    });
    controlled.request.onsuccess?.({} as Event);
  }

  function fail(operation: ControlledIndexedDBOperation, error: Error): void {
    const controlled = take(operation);
    Object.defineProperty(controlled.request, "error", {
      configurable: true,
      value: error,
    });
    controlled.request.onerror?.({} as Event);
  }

  return {
    controls: {
      fail,
      pendingOperations: () => pending.map(({ operation }) => operation),
      succeed,
    },
    indexedDB: {
      open: () => controlledRequest("open", () => database),
    } as unknown as IDBFactory,
    records,
  };
}

export async function flushPromiseContinuations(): Promise<void> {
  await Promise.resolve();
}
