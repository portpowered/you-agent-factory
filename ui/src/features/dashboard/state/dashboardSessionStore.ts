import { create } from "zustand";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";

interface DashboardSessionStoreState {
  selectedSessionID: string;
  setSelectedSessionID: (sessionID: string) => void;
}

export const useDashboardSessionStore = create<DashboardSessionStoreState>((set) => ({
  selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
  setSelectedSessionID: (sessionID) => {
    set({
      selectedSessionID:
        sessionID.trim().length > 0 ? sessionID : DEFAULT_FACTORY_SESSION_ID,
    });
  },
}));
