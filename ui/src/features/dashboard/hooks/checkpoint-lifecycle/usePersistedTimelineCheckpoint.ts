import { useCallback, useEffect, useMemo, useRef } from "react";

import { normalizeStreamDerivedCacheIdentity } from "../../../timeline/lib/stream-derived-cache-identity";
import {
  persistTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../../timeline/state/timelineCheckpointPersistence";
import type { FactoryTimelineCheckpoint } from "../../../timeline/state/timeline/storeState";

export const TIMELINE_CHECKPOINT_DEBOUNCE_MS = 750;

type PersistCheckpoint = (
  indexedDB: IDBFactory,
  checkpoint: FactoryTimelineCheckpoint,
  streamIdentity: TimelineCheckpointStreamIdentity,
) => void | Promise<void>;

export interface TimelineCheckpointLifecycleDependencies {
  cancel: (handle: number) => void;
  persist: PersistCheckpoint;
  schedule: (callback: () => void, delay: number) => number;
}

const defaultDependencies: TimelineCheckpointLifecycleDependencies = {
  cancel: (handle) => window.clearTimeout(handle),
  persist: persistTimelineCheckpoint,
  schedule: (callback, delay) => window.setTimeout(callback, delay),
};

interface PendingTimelineCheckpoint {
  checkpoint: FactoryTimelineCheckpoint;
  indexedDB: IDBFactory;
  streamIdentity: TimelineCheckpointStreamIdentity;
  streamKey: string;
}

function timelineCheckpointStreamKey(
  identity: TimelineCheckpointStreamIdentity,
): string {
  return JSON.stringify([
    identity.backendScopeID,
    identity.factorySessionID,
    identity.logicalSessionKeyID,
    identity.streamGenerationID,
  ]);
}

export function usePersistedTimelineCheckpoint({
  checkpoint,
  checkpointsDisabled,
  dependencies = defaultDependencies,
  streamIdentity,
  syncIdentity,
}: {
  checkpoint: FactoryTimelineCheckpoint | undefined;
  checkpointsDisabled: boolean;
  dependencies?: TimelineCheckpointLifecycleDependencies;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
  syncIdentity?: FactoryTimelineCheckpoint["syncIdentity"];
}) {
  const pendingCheckpointRef = useRef<PendingTimelineCheckpoint | null>(null);
  const persistHandleRef = useRef<number | null>(null);
  const { cancel, persist, schedule } = dependencies;
  const backendScopeID = streamIdentity?.backendScopeID;
  const factorySessionID = streamIdentity?.factorySessionID;
  const logicalSessionKeyID = streamIdentity?.logicalSessionKeyID;
  const streamGenerationID = streamIdentity?.streamGenerationID;
  const normalizedStreamIdentity = useMemo(
    () =>
      normalizeStreamDerivedCacheIdentity({
        backendScopeID,
        factorySessionID,
        logicalSessionKeyID,
        streamGenerationID,
      }),
    [backendScopeID, factorySessionID, logicalSessionKeyID, streamGenerationID],
  );
  const streamKey = normalizedStreamIdentity
    ? timelineCheckpointStreamKey(normalizedStreamIdentity)
    : null;

  const detachPendingCheckpoint = useCallback(
    (expectedStreamKey?: string): PendingTimelineCheckpoint | null => {
      const pending = pendingCheckpointRef.current;
      if (
        !pending ||
        (expectedStreamKey && pending.streamKey !== expectedStreamKey)
      ) {
        return null;
      }
      pendingCheckpointRef.current = null;
      if (persistHandleRef.current !== null) {
        cancel(persistHandleRef.current);
        persistHandleRef.current = null;
      }
      return pending;
    },
    [cancel],
  );

  const flushPendingCheckpoint = useCallback(
    (expectedStreamKey?: string) => {
      const pending = detachPendingCheckpoint(expectedStreamKey);
      if (!pending) return;
      void persist(
        pending.indexedDB,
        pending.checkpoint,
        pending.streamIdentity,
      );
    },
    [detachPendingCheckpoint, persist],
  );

  useEffect(() => {
    if (
      typeof window === "undefined" ||
      checkpointsDisabled ||
      !checkpoint ||
      !normalizedStreamIdentity ||
      !streamKey
    ) {
      detachPendingCheckpoint();
      return;
    }

    detachPendingCheckpoint();
    const pending = {
      checkpoint: {
        ...checkpoint,
        ...(syncIdentity ? { syncIdentity } : {}),
      },
      indexedDB: window.indexedDB,
      streamIdentity: normalizedStreamIdentity,
      streamKey,
    } satisfies PendingTimelineCheckpoint;
    pendingCheckpointRef.current = pending;
    persistHandleRef.current = schedule(() => {
      if (pendingCheckpointRef.current !== pending) return;
      flushPendingCheckpoint(pending.streamKey);
    }, TIMELINE_CHECKPOINT_DEBOUNCE_MS);
  }, [
    checkpoint,
    checkpointsDisabled,
    detachPendingCheckpoint,
    flushPendingCheckpoint,
    normalizedStreamIdentity,
    schedule,
    streamKey,
    syncIdentity,
  ]);

  useEffect(() => {
    if (!streamKey) return;
    return () => flushPendingCheckpoint(streamKey);
  }, [flushPendingCheckpoint, streamKey]);

  useEffect(() => {
    if (typeof window === "undefined" || typeof document === "undefined") {
      return;
    }

    const handlePageHide = () => flushPendingCheckpoint();
    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden") flushPendingCheckpoint();
    };

    window.addEventListener("pagehide", handlePageHide);
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      window.removeEventListener("pagehide", handlePageHide);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      flushPendingCheckpoint();
    };
  }, [flushPendingCheckpoint]);
}
