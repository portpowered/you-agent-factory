import { type ReactNode, useMemo } from "react";
import {
  DEFAULT_FACTORY_SESSION_ID,
  isDefaultFactorySessionID,
} from "../api/session-routing";
import { buildSessionScope } from "../api/session-scope";
import { DashboardSessionScopeProvider } from "../features/dashboard/session/dashboard-session-provider";
// biome-ignore lint/style/noRestrictedImports: the shared test harness intentionally projects controlled store changes without production discovery IO.
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";

export interface DashboardSessionTestProviderProps {
  children: ReactNode;
  paused?: boolean;
  sessionID?: string | null;
}

export function DashboardSessionTestProvider({
  children,
  paused = false,
  sessionID = DEFAULT_FACTORY_SESSION_ID,
}: DashboardSessionTestProviderProps) {
  const pausedSessionIDs = useMemo(
    () => (paused && sessionID != null ? [sessionID] : []),
    [paused, sessionID],
  );
  const scope = useMemo(
    () => buildSessionScope(sessionID, pausedSessionIDs),
    [pausedSessionIDs, sessionID],
  );

  return (
    <DashboardSessionScopeProvider scope={scope}>
      {children}
    </DashboardSessionScopeProvider>
  );
}

export function DashboardSessionStoreTestProvider({
  children,
  resolvedDefaultSessionID,
}: Pick<DashboardSessionTestProviderProps, "children"> & {
  resolvedDefaultSessionID?: string;
}) {
  const selectedSessionID = useDashboardSessionStore(
    (state) => state.selectedSessionID,
  );
  const pausedSessionIDs = useDashboardSessionStore(
    (state) => state.pausedSessionIDs,
  );
  const scope = useMemo(() => {
    const resolvesDefault =
      resolvedDefaultSessionID != null &&
      isDefaultFactorySessionID(selectedSessionID);
    return buildSessionScope(
      resolvesDefault ? resolvedDefaultSessionID : selectedSessionID,
      pausedSessionIDs,
      resolvesDefault,
    );
  }, [pausedSessionIDs, resolvedDefaultSessionID, selectedSessionID]);

  return (
    <DashboardSessionScopeProvider scope={scope}>
      {children}
    </DashboardSessionScopeProvider>
  );
}
