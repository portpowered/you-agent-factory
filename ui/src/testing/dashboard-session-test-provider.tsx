import { type ReactNode, useEffect } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import { useDashboardSessionStore } from "../features/dashboard/state/dashboardSessionStore";

export interface DashboardSessionTestProviderProps {
  children: ReactNode;
  sessionID?: string | null;
}

export function seedDashboardSessionForTest(
  sessionID: string | null | undefined = DEFAULT_FACTORY_SESSION_ID,
): void {
  useDashboardSessionStore.getState().setSelectedSessionID(sessionID);
}

export function DashboardSessionTestProvider({
  children,
  sessionID = DEFAULT_FACTORY_SESSION_ID,
}: DashboardSessionTestProviderProps) {
  useEffect(() => {
    seedDashboardSessionForTest(sessionID);
  }, [sessionID]);

  return children;
}
