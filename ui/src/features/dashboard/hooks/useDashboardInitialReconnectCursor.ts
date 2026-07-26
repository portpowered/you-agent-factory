import { useEffect, useMemo, useRef } from "react";

import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  type FactoryTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
} from "../../timeline/public/checkpoint-reconnect";
import {
  dashboardSessionKey,
  shouldResumeFromPersistedCheckpoint,
} from "../lib/dashboard-session-key";

export function useDashboardInitialReconnectCursor({
  persistedCheckpoint,
  refreshToken,
  sessionID,
}: {
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  refreshToken: number;
  sessionID: string | null;
}): FactoryEventReconnectCursor | undefined {
  const previousSessionKeyRef = useRef<string | null>(null);
  const sessionKey = useMemo(
    () => dashboardSessionKey(sessionID, refreshToken),
    [refreshToken, sessionID],
  );
  const reconnectCursor = useMemo(
    () =>
      resolveDashboardInitialReconnectCursor({
        persistedCheckpoint,
        previousSessionKey: previousSessionKeyRef.current,
        refreshToken,
        sessionID,
      }),
    [persistedCheckpoint, refreshToken, sessionID],
  );

  useEffect(() => {
    previousSessionKeyRef.current = sessionKey;
  }, [sessionKey]);

  return reconnectCursor;
}

export function resolveDashboardInitialReconnectCursor({
  persistedCheckpoint,
  previousSessionKey,
  refreshToken,
  sessionID,
}: {
  persistedCheckpoint: FactoryTimelineCheckpoint | null;
  previousSessionKey: string | null;
  refreshToken: number;
  sessionID: string | null;
}): FactoryEventReconnectCursor | undefined {
  return shouldResumeFromPersistedCheckpoint({
    previousSessionKey,
    refreshToken,
    sessionID,
  })
    ? reconnectCursorFromCheckpoint(persistedCheckpoint)
    : undefined;
}
