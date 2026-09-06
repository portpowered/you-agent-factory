const WORKER_SESSION_TIMELINE_SELECTION_KEY_PREFIX =
  "you-agent-factory.worker-session-timeline.selected-worker-session";

type StorageReader = Pick<Storage, "getItem">;
type StorageWriter = Pick<Storage, "removeItem" | "setItem">;

/**
 * Builds a scope-specific browser-storage key. Session storage keeps a
 * selection through a tab reload and disposable server restart while the
 * Factory Session/Work tuple prevents one dashboard scope from selecting a
 * Worker Session from another scope.
 */
export function getWorkerSessionTimelineSelectionStorageKey(
  factorySessionID: string | null,
  workID: string | null,
): string | null {
  if (factorySessionID === null || workID === null) {
    return null;
  }
  return [
    WORKER_SESSION_TIMELINE_SELECTION_KEY_PREFIX,
    encodeURIComponent(factorySessionID),
    encodeURIComponent(workID),
  ].join(":");
}

export function readWorkerSessionTimelineSelection(
  storageKey: string | null,
  storage?: StorageReader | null,
): string | null {
  if (storageKey === null) {
    return null;
  }
  const target = storage ?? getSessionStorage();
  if (target === null) {
    return null;
  }
  try {
    const value = target.getItem(storageKey);
    return value !== null && value.length > 0 ? value : null;
  } catch {
    return null;
  }
}

export function writeWorkerSessionTimelineSelection(
  storageKey: string | null,
  workerSessionID: string | null,
  storage?: StorageWriter | null,
): void {
  if (storageKey === null) {
    return;
  }
  const target = storage ?? getSessionStorage();
  if (target === null) {
    return;
  }
  try {
    if (workerSessionID === null || workerSessionID.length === 0) {
      target.removeItem(storageKey);
      return;
    }
    target.setItem(storageKey, workerSessionID);
  } catch {
    // Browser storage can be disabled or full; selection remains in memory.
  }
}

function getSessionStorage(): Storage | null {
  if (typeof window === "undefined") {
    return null;
  }
  try {
    return window.sessionStorage;
  } catch {
    return null;
  }
}
