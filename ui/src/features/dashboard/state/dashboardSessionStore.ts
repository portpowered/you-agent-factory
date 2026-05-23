import { create } from "zustand";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";

interface DashboardSessionStoreState {
  pausedSessionIDs: string[];
  setSessionPaused: (sessionID: string, paused: boolean) => void;
  selectedSessionID: string | null;
  setSelectedSessionID: (sessionID: string | null) => void;
}

export const useDashboardSessionStore = create<DashboardSessionStoreState>((set) => ({
  pausedSessionIDs: [],
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
  selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
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
}));
