import { create } from "zustand";

interface DashboardBentoState {
  incrementRefreshToken: () => void;
  refreshToken: number;
  resetSelectedTraceID: () => void;
  selectedTraceID: string | null;
  setSelectedTraceID: (traceID: string | null) => void;
}

export const useDashboardBentoStore = create<DashboardBentoState>((set) => ({
  incrementRefreshToken: () => {
    set((state) => ({ refreshToken: state.refreshToken + 1 }));
  },
  refreshToken: 0,
  resetSelectedTraceID: () => {
    set({ selectedTraceID: null });
  },
  selectedTraceID: null,
  setSelectedTraceID: (traceID) => {
    set({ selectedTraceID: traceID });
  },
}));

