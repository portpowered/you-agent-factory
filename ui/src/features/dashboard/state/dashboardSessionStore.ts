import { create } from "zustand";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";

interface DashboardSessionStoreState {
  pausedSessionIDs: string[];
  sessionTabOrder: string[];
  setSessionPaused: (sessionID: string, paused: boolean) => void;
  setSessionTabOrder: (sessionIDs: string[]) => void;
  resolveSessionIdentity: (
    selectorSessionID: string,
    resolvedSessionID: string,
    discoveredSessionIDs: string[],
  ) => void;
  selectedSessionID: string | null;
  setSelectedSessionID: (sessionID: string | null) => void;
}

const DASHBOARD_SESSION_STORE_DEFAULTS = {
  pausedSessionIDs: [],
  sessionTabOrder: [],
  selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
} satisfies Pick<
  DashboardSessionStoreState,
  "pausedSessionIDs" | "selectedSessionID" | "sessionTabOrder"
>;

export const useDashboardSessionStore = create<DashboardSessionStoreState>(
  (set) => ({
    ...DASHBOARD_SESSION_STORE_DEFAULTS,
    setSessionPaused: (sessionID, paused) => {
      const normalizedSessionID = sessionID.trim();
      if (normalizedSessionID.length === 0) {
        return;
      }

      set((current) => ({
        pausedSessionIDs: paused
          ? current.pausedSessionIDs.includes(normalizedSessionID)
            ? current.pausedSessionIDs
            : [...current.pausedSessionIDs, normalizedSessionID]
          : current.pausedSessionIDs.filter(
              (currentSessionID) => currentSessionID !== normalizedSessionID,
            ),
      }));
    },
    setSessionTabOrder: (sessionIDs) => {
      const normalizedOrder: string[] = [];

      for (const sessionID of sessionIDs) {
        const normalizedSessionID = sessionID.trim();
        if (
          normalizedSessionID.length > 0 &&
          !normalizedOrder.includes(normalizedSessionID)
        ) {
          normalizedOrder.push(normalizedSessionID);
        }
      }

      set({
        sessionTabOrder: normalizedOrder,
      });
    },
    resolveSessionIdentity: (
      selectorSessionID,
      resolvedSessionID,
      discoveredSessionIDs,
    ) => {
      const normalizedSelector = selectorSessionID.trim();
      const normalizedResolved = resolvedSessionID.trim();
      if (normalizedSelector.length === 0 || normalizedResolved.length === 0) {
        return;
      }

      set((current) => {
        const nextOrder: string[] = [];
        for (const sessionID of [
          ...current.sessionTabOrder,
          ...discoveredSessionIDs,
        ]) {
          const normalizedSessionID = sessionID.trim();
          const canonicalSessionID =
            normalizedSessionID === normalizedSelector
              ? normalizedResolved
              : normalizedSessionID;
          if (
            canonicalSessionID.length > 0 &&
            !nextOrder.includes(canonicalSessionID)
          ) {
            nextOrder.push(canonicalSessionID);
          }
        }

        return {
          selectedSessionID:
            current.selectedSessionID == null ||
            current.selectedSessionID.trim().length === 0 ||
            current.selectedSessionID === normalizedSelector
              ? normalizedResolved
              : current.selectedSessionID,
          sessionTabOrder: nextOrder,
        };
      });
    },
    setSelectedSessionID: (sessionID) => {
      set({
        selectedSessionID:
          sessionID == null
            ? null
            : sessionID.trim().length > 0
              ? sessionID
              : DEFAULT_FACTORY_SESSION_ID,
      });
    },
  }),
);

export function resetDashboardSessionStore(): void {
  useDashboardSessionStore.setState(DASHBOARD_SESSION_STORE_DEFAULTS);
}
