import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
} from "react";

import {
  type FactorySessionSummary,
  listFactorySessions,
} from "../../../api/factory-sessions";
import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../../../api/session-routing";
import {
  buildSessionScope,
  type SessionScope,
} from "../../../api/session-scope";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";

const DashboardSessionContext = createContext<SessionScope | null>(null);

export interface DashboardSessionScopeProviderProps {
  children: ReactNode;
  scope: SessionScope;
}

export function DashboardSessionScopeProvider({
  children,
  scope,
}: DashboardSessionScopeProviderProps) {
  return (
    <DashboardSessionContext.Provider value={scope}>
      {children}
    </DashboardSessionContext.Provider>
  );
}

export interface DashboardSessionProviderProps {
  children: ReactNode;
  renderDiscoveryState?: (state: DashboardSessionDiscoveryState) => ReactNode;
}

export type DashboardSessionDiscoveryState =
  | { status: "loading" }
  | { status: "empty" }
  | { error: unknown; retry: () => void; status: "error" };

export function DashboardSessionProvider({
  children,
  renderDiscoveryState,
}: DashboardSessionProviderProps) {
  const inheritedScope = useContext(DashboardSessionContext);

  if (inheritedScope !== null) {
    return children;
  }

  return (
    <DashboardSessionDiscoveryProvider
      renderDiscoveryState={renderDiscoveryState}
    >
      {children}
    </DashboardSessionDiscoveryProvider>
  );
}

function DashboardSessionDiscoveryProvider({
  children,
  renderDiscoveryState,
}: DashboardSessionProviderProps) {
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const pausedSessionIDs = useDashboardSessionStore(
    (state) => state.pausedSessionIDs,
  );
  const resolveSessionIdentity = useDashboardSessionStore(
    (state) => state.resolveSessionIdentity,
  );
  const sessionsQuery = useQuery({
    queryKey: FACTORY_SESSIONS_QUERY_KEY,
    queryFn: ({ signal }) => listFactorySessions({ signal }),
  });
  const requiresDefaultResolution =
    isDefaultFactorySessionID(selectedSessionID);
  const resolvedDefaultSession = useMemo(
    () => findResolvedDefaultSession(sessionsQuery.data),
    [sessionsQuery.data],
  );
  const resolvedSelectedSessionID =
    requiresDefaultResolution && resolvedDefaultSession
      ? resolvedDefaultSession.id
      : selectedSessionID;

  useEffect(() => {
    if (!resolvedDefaultSession) {
      return;
    }
    resolveSessionIdentity(
      DEFAULT_FACTORY_SESSION_ID,
      resolvedDefaultSession.id,
      (sessionsQuery.data ?? []).map((session) => session.id),
    );
  }, [resolveSessionIdentity, resolvedDefaultSession, sessionsQuery.data]);

  const scope = useMemo(
    () =>
      buildSessionScope(
        resolvedSelectedSessionID,
        pausedSessionIDs,
        resolvedDefaultSession?.id === resolvedSelectedSessionID,
      ),
    [pausedSessionIDs, resolvedDefaultSession, resolvedSelectedSessionID],
  );

  if (requiresDefaultResolution && !resolvedDefaultSession) {
    if (sessionsQuery.isPending) {
      return renderDiscoveryState?.({ status: "loading" }) ?? null;
    }
    if (sessionsQuery.isError) {
      return (
        renderDiscoveryState?.({
          error: sessionsQuery.error,
          retry: () => {
            void sessionsQuery.refetch();
          },
          status: "error",
        }) ?? null
      );
    }
    return renderDiscoveryState?.({ status: "empty" }) ?? null;
  }

  return (
    <DashboardSessionScopeProvider scope={scope}>
      {children}
    </DashboardSessionScopeProvider>
  );
}

function findResolvedDefaultSession(
  sessions: FactorySessionSummary[] | undefined,
): FactorySessionSummary | null {
  return (
    sessions?.find(
      (session) =>
        session.isDefault &&
        session.target.kind === "default" &&
        session.id.trim().length > 0 &&
        !isDefaultFactorySessionID(session.id),
    ) ?? null
  );
}

export function useDashboardSession(): SessionScope {
  const scope = useContext(DashboardSessionContext);

  if (scope === null) {
    throw new Error(
      "useDashboardSession must be used within DashboardSessionProvider.",
    );
  }

  return scope;
}

export function useRemapDashboardSelectedSession(): (
  sessionID: string,
) => void {
  const queryClient = useQueryClient();
  const remapSelectedSessionIdentity = useDashboardSessionStore(
    (state) => state.remapSelectedSessionIdentity,
  );
  return useCallback(
    (sessionID: string) => {
      const supersededSessionID =
        useDashboardSessionStore.getState().selectedSessionID;
      if (supersededSessionID != null && supersededSessionID !== sessionID) {
        queryClient.setQueryData<FactorySessionSummary[]>(
          FACTORY_SESSIONS_QUERY_KEY,
          (sessions) =>
            sessions?.map((session) =>
              session.id === supersededSessionID
                ? { ...session, id: sessionID }
                : session,
            ),
        );
      }
      remapSelectedSessionIdentity(sessionID);
    },
    [queryClient, remapSelectedSessionIdentity],
  );
}
