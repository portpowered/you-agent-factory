import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";

import type { FactoryEventReconnectCursor } from "../../../../api/events";
import {
  clearTimelineCheckpointsForSession,
  deletePersistedTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../../timeline/public/checkpoint-persistence";
import type { FactoryTimelineCheckpoint } from "../../../timeline/public/store";
import {
  isDefaultToRuntimeSessionAliasRemap,
  recoverDashboardSessionScopedState,
} from "../../lib/dashboard-session-lifecycle";
import {
  checkpointSyncIdentityFromPreflight,
  type DashboardSessionRecoveryState,
} from "../../lib/preflight/dashboard-session-sync-preflight";
import {
  type DashboardCheckpointPreflightResolution,
  resolveDashboardCheckpointPreflight,
} from "../../lib/preflight/resolve-dashboard-checkpoint-preflight";
import {
  correlationTokenForIdentityScope,
  createSessionPersistenceCorrelationToken,
  recordSessionPersistenceDiagnostic,
  sessionPersistenceDiagnostic,
} from "../../lib/session-persistence/diagnostics";
import { useRemapDashboardSelectedSession } from "../../session/dashboard-session-provider";
import { useDashboardStreamStore } from "../../state/dashboardStreamStore";

export interface UseDashboardCheckpointPreflightResult {
  checkpointHydrated: boolean;
  cursorFreeReplayCorrelationToken: string | null;
  initialReconnectCursor?: FactoryEventReconnectCursor;
  preflightError: Error | null;
  preflightRecovery: DashboardSessionRecoveryState | null;
  preflightReady: boolean;
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  resolvedSessionID: string | null;
  streamIdentity: TimelineCheckpointStreamIdentity | null;
}

function resetDashboardCheckpointPreflightState(
  setCheckpointHydratedKey: (value: string | null) => void,
  setCursorFreeReplayCorrelationToken: (value: string | null) => void,
  setPersistedCheckpoint: (value: FactoryTimelineCheckpoint | null) => void,
  setPreflightError: (value: Error | null) => void,
  setPreflightRecovery: (value: DashboardSessionRecoveryState | null) => void,
  setPreflightReadyKey: (value: string | null) => void,
  setInitialReconnectCursor: (
    value: FactoryEventReconnectCursor | undefined,
  ) => void,
  setResolvedSessionID: (value: string | null) => void,
  setStreamIdentity: (value: TimelineCheckpointStreamIdentity | null) => void,
): void {
  setCheckpointHydratedKey(null);
  setCursorFreeReplayCorrelationToken(null);
  setPersistedCheckpoint(null);
  setPreflightError(null);
  setPreflightRecovery(null);
  setPreflightReadyKey(null);
  setInitialReconnectCursor(undefined);
  setResolvedSessionID(null);
  setStreamIdentity(null);
}

function recordCheckpointLookup(
  resolution: DashboardCheckpointPreflightResolution,
): void {
  if (!resolution.checkpointLookupOutcome) return;
  try {
    const correlationToken =
      "streamIdentity" in resolution
        ? correlationTokenForIdentityScope(resolution.streamIdentity)
        : createSessionPersistenceCorrelationToken(
            resolution.requestedSessionId,
          );
    recordSessionPersistenceDiagnostic(
      sessionPersistenceDiagnostic(
        resolution.checkpointLookupOutcome,
        correlationToken,
      ),
    );
  } catch {
    // Diagnostics are best effort and cannot affect checkpoint recovery.
  }
}

function recordIdentityOutcome(
  resolution: DashboardCheckpointPreflightResolution,
): void {
  if (!("streamIdentity" in resolution)) return;
  try {
    const correlationToken = correlationTokenForIdentityScope(
      resolution.streamIdentity,
    );
    if (resolution.identityRejectionDetail) {
      recordSessionPersistenceDiagnostic(
        sessionPersistenceDiagnostic(
          "identity_rejected",
          correlationToken,
          resolution.identityRejectionDetail,
        ),
      );
    }
    if (
      resolution.kind === "remap" &&
      !isDefaultToRuntimeSessionAliasRemap(
        resolution.requestedSessionId,
        resolution.resolvedSessionId,
      )
    ) {
      recordSessionPersistenceDiagnostic(
        sessionPersistenceDiagnostic("logical_remap", correlationToken),
      );
    }
  } catch {
    // Diagnostics are best effort and cannot affect identity recovery.
  }
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: this hook deliberately owns the single guarded apply boundary for all preflight mutations.
export function useDashboardCheckpointPreflight({
  checkpointHydrationKey,
  checkpointsDisabled,
  rawSessionID,
  restoreCheckpoint,
}: {
  checkpointHydrationKey: string | null;
  checkpointsDisabled: boolean;
  rawSessionID: string | null;
  refreshToken: number;
  restoreCheckpoint: (
    streamIdentity: TimelineCheckpointStreamIdentity,
    checkpoint: FactoryTimelineCheckpoint,
  ) => void;
}): UseDashboardCheckpointPreflightResult {
  const queryClient = useQueryClient();
  const remapSelectedSessionID = useRemapDashboardSelectedSession();
  const setStreamState = useDashboardStreamStore(
    (state) => state.setStreamState,
  );
  const [checkpointHydratedKey, setCheckpointHydratedKey] = useState<
    string | null
  >(null);
  const [
    cursorFreeReplayCorrelationToken,
    setCursorFreeReplayCorrelationToken,
  ] = useState<string | null>(null);
  const [preflightReadyKey, setPreflightReadyKey] = useState<string | null>(
    null,
  );
  const [preflightError, setPreflightError] = useState<Error | null>(null);
  const [preflightRecovery, setPreflightRecovery] =
    useState<DashboardSessionRecoveryState | null>(null);
  const [persistedCheckpoint, setPersistedCheckpoint] =
    useState<FactoryTimelineCheckpoint | null>(null);
  const [initialReconnectCursor, setInitialReconnectCursor] = useState<
    FactoryEventReconnectCursor | undefined
  >(undefined);
  const [resolvedSessionID, setResolvedSessionID] = useState<string | null>(
    null,
  );
  const [streamIdentity, setStreamIdentity] =
    useState<TimelineCheckpointStreamIdentity | null>(null);
  const invocationRevision = useRef(0);

  const checkpointHydrated = checkpointHydratedKey === checkpointHydrationKey;
  const preflightReady = preflightReadyKey === checkpointHydrationKey;

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: the effect owns one guarded preflight invocation and its apply boundary.
  useEffect(() => {
    const revision = ++invocationRevision.current;
    const abortController = new AbortController();
    const isActive = (requestedSessionId: string): boolean =>
      invocationRevision.current === revision &&
      requestedSessionId === rawSessionID;

    resetDashboardCheckpointPreflightState(
      setCheckpointHydratedKey,
      setCursorFreeReplayCorrelationToken,
      setPersistedCheckpoint,
      setPreflightError,
      setPreflightRecovery,
      setPreflightReadyKey,
      setInitialReconnectCursor,
      setResolvedSessionID,
      setStreamIdentity,
    );

    if (
      checkpointHydrationKey == null ||
      !rawSessionID ||
      typeof window === "undefined" ||
      checkpointsDisabled
    ) {
      setResolvedSessionID(rawSessionID);
      setPreflightReadyKey(checkpointHydrationKey);
      setCheckpointHydratedKey(checkpointHydrationKey);
      return;
    }

    void (async () => {
      try {
        const resolution = await resolveDashboardCheckpointPreflight({
          indexedDB: window.indexedDB,
          requestedSessionId: rawSessionID,
          signal: abortController.signal,
        });
        await applyResolution(resolution, isActive, abortController.signal);
      } catch (failure: unknown) {
        if (!abortController.signal.aborted) {
          throw failure;
        }
      }
    })();

    return () => {
      invocationRevision.current += 1;
      abortController.abort();
    };

    async function applyResolution(
      resolution: DashboardCheckpointPreflightResolution,
      remainsActive: (requestedSessionId: string) => boolean,
      signal: AbortSignal,
    ): Promise<void> {
      if (!remainsActive(resolution.requestedSessionId)) return;

      if (resolution.clearRequestedSessionCheckpoint) {
        if (resolution.checkpointToDelete) {
          await deletePersistedTimelineCheckpoint(
            window.indexedDB,
            resolution.checkpointToDelete,
            { signal },
          );
        } else {
          await clearTimelineCheckpointsForSession(
            window.indexedDB,
            resolution.requestedSessionId,
            { signal },
          );
        }
        if (!remainsActive(resolution.requestedSessionId)) return;
        recoverDashboardSessionScopedState(
          queryClient,
          resolution.requestedSessionId,
          () => {},
        );
      }

      if (!remainsActive(resolution.requestedSessionId)) return;
      setCheckpointHydratedKey(checkpointHydrationKey);
      recordCheckpointLookup(resolution);

      if (resolution.kind === "error") {
        setPreflightError(resolution.error);
        setStreamState({
          message: resolution.error.message,
          status: "offline",
        });
        return;
      }
      if (resolution.kind === "recovery") {
        setPreflightRecovery({
          reasonCode: resolution.reasonCode,
          requestedSessionId: resolution.requestedSessionId,
        });
        setPreflightReadyKey(checkpointHydrationKey);
        return;
      }

      if (
        resolution.kind === "remap" &&
        !isDefaultToRuntimeSessionAliasRemap(
          resolution.requestedSessionId,
          resolution.resolvedSessionId,
        )
      ) {
        remapSelectedSessionID(resolution.resolvedSessionId);
      }
      recordIdentityOutcome(resolution);
      if (resolution.kind === "resume" && resolution.staleCursorDetected) {
        try {
          const correlationToken = correlationTokenForIdentityScope(
            resolution.streamIdentity,
          );
          recordSessionPersistenceDiagnostic(
            sessionPersistenceDiagnostic("stale_cursor", correlationToken),
          );
          setCursorFreeReplayCorrelationToken(correlationToken);
        } catch {
          // Diagnostics are best effort and cannot affect cursor recovery.
        }
      }
      if (resolution.kind === "resume" && resolution.checkpoint) {
        restoreCheckpoint(resolution.streamIdentity, {
          ...resolution.checkpoint,
          syncIdentity: checkpointSyncIdentityFromPreflight(
            resolution.streamIdentity,
          ),
        });
      }
      setResolvedSessionID(resolution.resolvedSessionId);
      setStreamIdentity(resolution.streamIdentity);
      setInitialReconnectCursor(
        resolution.kind === "resume" ? resolution.reconnectCursor : undefined,
      );
      setPersistedCheckpoint(
        resolution.kind === "resume" ? resolution.checkpoint : null,
      );
      setPreflightReadyKey(checkpointHydrationKey);
    }
  }, [
    checkpointHydrationKey,
    checkpointsDisabled,
    queryClient,
    rawSessionID,
    remapSelectedSessionID,
    restoreCheckpoint,
    setStreamState,
  ]);

  return {
    checkpointHydrated,
    cursorFreeReplayCorrelationToken,
    initialReconnectCursor,
    preflightError,
    preflightRecovery,
    preflightReady,
    persistedCheckpoint,
    resolvedSessionID,
    streamIdentity,
  };
}
