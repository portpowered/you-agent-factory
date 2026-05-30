import { type ReactNode, useMemo } from "react";
import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import { buildSessionScope } from "../api/session-scope";
import { DashboardSessionScopeProvider } from "../features/dashboard/session/dashboard-session-provider";

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
