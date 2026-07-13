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
  isAborted: () => boolean;
  operation: ControlledIndexedDBOperation;
  request: IDBRequest<T>;
  result: () => T;
}

interface ControlledTransaction {
  commitActions: Array<() => void>;
  transaction: IDBTransaction;
}

function createTransactionControls(pending: ControlledTransaction[]) {
  function take(ordinal = 0): ControlledTransaction {
    const controlled = pending[ordinal];
    if (!controlled) throw new Error("no pending IndexedDB transaction");
    pending.splice(ordinal, 1);
    return controlled;
  }

  return {
    abort(error?: Error, ordinal = 0): void {
      const { transaction } = take(ordinal);
      Object.defineProperty(transaction, "error", {
        configurable: true,
        value: error ?? null,
      });
      transaction.onabort?.({} as Event);
    },
    complete(ordinal = 0): void {
      const controlled = take(ordinal);
      for (const commit of controlled.commitActions) commit();
      controlled.transaction.oncomplete?.({} as Event);
    },
    fail(error: Error, ordinal = 0): void {
      const { transaction } = take(ordinal);
      Object.defineProperty(transaction, "error", {
        configurable: true,
        value: error,
      });
      transaction.onerror?.({} as Event);
    },
  };
}

function createRequestControls(pending: ControlledRequest<unknown>[]) {
  function take(
    operation: ControlledIndexedDBOperation,
    ordinal = 0,
  ): ControlledRequest<unknown> {
    const matchingIndexes = pending.flatMap((request, index) =>
      request.operation === operation ? [index] : [],
    );
    const index = matchingIndexes[ordinal] ?? -1;
    const controlled = pending[index];
    if (index < 0 || !controlled) {
      throw new Error(`no pending IndexedDB ${operation} request`);
    }
    pending.splice(index, 1);
    return controlled;
  }

  return {
    fail(operation: ControlledIndexedDBOperation, error: Error, ordinal = 0) {
      const controlled = take(operation, ordinal);
      Object.defineProperty(controlled.request, "error", {
        configurable: true,
        value: error,
      });
      controlled.request.onerror?.({} as Event);
    },
    succeed(operation: ControlledIndexedDBOperation, ordinal = 0): void {
      const controlled = take(operation, ordinal);
      if (!controlled.isAborted()) controlled.beforeSuccess?.();
      Object.defineProperty(controlled.request, "result", {
        configurable: true,
        value: controlled.result(),
      });
      controlled.request.onsuccess?.({} as Event);
    },
  };
}

export function createControlledIndexedDBTestDouble<
  RecordType extends StoredRecord,
>() {
  const records = new Map<string, RecordType>();
  const pending: ControlledRequest<unknown>[] = [];
  const pendingTransactions: ControlledTransaction[] = [];
  const requestControls = createRequestControls(pending);
  const transactionControls = createTransactionControls(pendingTransactions);
  let closedDatabaseCount = 0;

  function controlledRequest<T>(
    operation: ControlledIndexedDBOperation,
    result: () => T,
    beforeSuccess?: () => void,
    isAborted: () => boolean = () => false,
  ): IDBRequest<T> {
    const request = {
      error: null,
      onerror: null,
      onsuccess: null,
      onupgradeneeded: null,
      result: undefined,
    } as unknown as IDBRequest<T>;
    pending.push({
      beforeSuccess,
      isAborted,
      operation,
      request,
      result,
    } as ControlledRequest<unknown>);
    return request;
  }

  const database = {
    close: () => {
      closedDatabaseCount += 1;
    },
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => {
      let aborted = false;
      const commitActions: Array<() => void> = [];
      const transaction = {
        error: null,
        onabort: null,
        oncomplete: null,
        onerror: null,
        abort: () => {
          aborted = true;
        },
        objectStore: () => ({
          delete: (key: string) =>
            controlledRequest(
              "delete",
              () => undefined,
              () => records.delete(key),
              () => aborted,
            ),
          get: (key: string) =>
            controlledRequest(
              "get",
              () => records.get(key),
              undefined,
              () => aborted,
            ),
          getAll: () =>
            controlledRequest(
              "getAll",
              () => [...records.values()],
              undefined,
              () => aborted,
            ),
          put: (value: RecordType) =>
            controlledRequest(
              "put",
              () => value.storageKey ?? "",
              () => {
                commitActions.push(() => {
                  if (value.storageKey) {
                    records.set(value.storageKey, value);
                  }
                });
              },
              () => aborted,
            ),
        }),
      } as unknown as IDBTransaction;
      pendingTransactions.push({ commitActions, transaction });
      return transaction;
    },
  };

  return {
    controls: {
      abortTransaction: transactionControls.abort,
      closedDatabaseCount: () => closedDatabaseCount,
      completeTransaction: transactionControls.complete,
      fail: requestControls.fail,
      failTransaction: transactionControls.fail,
      pendingOperations: () => pending.map(({ operation }) => operation),
      pendingTransactionCount: () => pendingTransactions.length,
      succeed: requestControls.succeed,
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
