import { useEffect, useMemo, useRef } from "react";

import type { FactoryEventReconnectCursor } from "../../../api/events";
import {
  type FactoryTimelineCheckpoint,
  reconnectCursorFromCheckpoint,
} from "../../timeline/public";
import {
  dashboardSessionKey,
  shouldResumeFromPersistedCheckpoint,
} from "../lib/dashboard-session-lifecycle";

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
  const shouldResumeFromCheckpoint = useMemo(
    () =>
      shouldResumeFromPersistedCheckpoint({
        previousSessionKey: previousSessionKeyRef.current,
        refreshToken,
        sessionID,
      }),
    [refreshToken, sessionID],
  );

  useEffect(() => {
    previousSessionKeyRef.current = sessionKey;
  }, [sessionKey]);

  return useMemo(
    () =>
      shouldResumeFromCheckpoint
        ? reconnectCursorFromCheckpoint(persistedCheckpoint)
        : undefined,
    [persistedCheckpoint, shouldResumeFromCheckpoint],
  );
}
