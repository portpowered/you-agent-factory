import { create } from "zustand";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";

interface DashboardSessionStoreState {
  selectedSessionID: string | null;
  setSelectedSessionID: (sessionID: string | null) => void;
}

export const useDashboardSessionStore = create<DashboardSessionStoreState>((set) => ({
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
