import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef } from "react";

import {
  dashboardSessionKey,
  resetDashboardSessionScopedState,
  shouldResetDashboardSessionScopedState,
} from "../lib/dashboard-session-lifecycle";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardStreamStore } from "../state/dashboardStreamStore";

export interface UseDashboardSessionLifecycleOptions {
  locale?: string | null;
  refreshToken?: number;
  sessionID: string | null;
}

export function useDashboardSessionLifecycle({
  locale,
  refreshToken = 0,
  sessionID,
}: UseDashboardSessionLifecycleOptions) {
  const queryClient = useQueryClient();
  const resetTimeline = useFactoryTimelineStore((state) => state.reset);
  const resetStreamState = useDashboardStreamStore((state) => state.resetStreamState);
  const lastSessionKeyRef = useRef<string | null>(null);

  const sessionKey = useMemo(
    () => dashboardSessionKey(sessionID, refreshToken),
    [refreshToken, sessionID],
  );

  const resetLocalizedSessionState = useCallback(() => {
    resetDashboardSessionScopedState(
      queryClient,
      resetStreamState,
      resetTimeline,
      locale,
    );
  }, [locale, queryClient, resetStreamState, resetTimeline]);

  useEffect(() => {
    const previousSessionKey = lastSessionKeyRef.current;
    const sessionChanged = sessionKey !== previousSessionKey;
    lastSessionKeyRef.current = sessionKey;

    if (!sessionChanged) {
      return;
    }

    if (
      !shouldResetDashboardSessionScopedState({
        previousSessionKey,
        refreshToken,
        sessionID,
      })
    ) {
      return;
    }

    resetLocalizedSessionState();
  }, [
    refreshToken,
    resetLocalizedSessionState,
    sessionID,
    sessionKey,
  ]);

  return useMemo(
    () => ({
      isSessionSelected: sessionID != null,
      sessionKey,
    }),
    [sessionID, sessionKey],
  );
}
