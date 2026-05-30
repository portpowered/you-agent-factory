import type { ReactNode } from "react";
import { createContext, useContext, useMemo } from "react";

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
}

export function DashboardSessionProvider({
  children,
}: DashboardSessionProviderProps) {
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const pausedSessionIDs = useDashboardSessionStore(
    (state) => state.pausedSessionIDs,
  );
  const scope = useMemo(
    () => buildSessionScope(selectedSessionID, pausedSessionIDs),
    [pausedSessionIDs, selectedSessionID],
  );

  return (
    <DashboardSessionScopeProvider scope={scope}>
      {children}
    </DashboardSessionScopeProvider>
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
