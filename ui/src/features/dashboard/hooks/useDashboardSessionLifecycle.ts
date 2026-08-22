import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { shouldResetDashboardRuntimeScopedState } from "../lib/backend-scope/backend-runtime-cache-scope";
import {
  dashboardSessionKey,
  type FactoryDefinitionQueryResetMode,
  resetDashboardSessionScopedState,
  sessionIDFromDashboardSessionKey,
  shouldResetDashboardSessionScopedState,
} from "../lib/dashboard-session-lifecycle";
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
  const resetSessionStreamState = useDashboardStreamStore(
    (state) => state.resetSessionStreamState,
  );
  const backendRuntimeCacheScope = useDashboardStreamStore(
    (state) => state.backendRuntimeCacheScope,
  );
  const lastSessionKeyRef = useRef<string | null>(null);
  const lastBackendScopeRef = useRef<string | null>(null);

  const sessionKey = useMemo(
    () => dashboardSessionKey(sessionID, refreshToken),
    [refreshToken, sessionID],
  );

  const resetLocalizedSessionState = useCallback(
    (factoryDefinitionQueryResetMode: FactoryDefinitionQueryResetMode) => {
      resetDashboardSessionScopedState(
        queryClient,
        (nextLocale) => {
          resetSessionStreamState(sessionID, nextLocale);
        },
        resetTimeline,
        locale,
        factoryDefinitionQueryResetMode,
      );
    },
    [locale, queryClient, resetSessionStreamState, resetTimeline, sessionID],
  );

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

    const previousSessionID =
      sessionIDFromDashboardSessionKey(previousSessionKey);
    resetLocalizedSessionState(
      previousSessionID !== null && previousSessionID === sessionID
        ? "invalidate"
        : "remove",
    );
  }, [refreshToken, resetLocalizedSessionState, sessionID, sessionKey]);

  useEffect(() => {
    const previousBackendScope = lastBackendScopeRef.current;
    const backendScopeChanged =
      backendRuntimeCacheScope !== previousBackendScope;
    lastBackendScopeRef.current = backendRuntimeCacheScope;

    if (!backendScopeChanged) {
      return;
    }

    if (
      !shouldResetDashboardRuntimeScopedState({
        previousBackendScope,
        backendRuntimeCacheScope,
      })
    ) {
      return;
    }

    resetLocalizedSessionState("remove");
  }, [backendRuntimeCacheScope, resetLocalizedSessionState]);

  return useMemo(
    () => ({
      isSessionSelected: sessionID != null,
      sessionKey,
    }),
    [sessionID, sessionKey],
  );
}
