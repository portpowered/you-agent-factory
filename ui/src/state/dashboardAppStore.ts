import { create } from "zustand";

interface DashboardAppState {
  closeExportDialog: () => void;
  incrementRefreshToken: () => void;
  isExportDialogOpen: boolean;
  openExportDialog: () => void;
  refreshToken: number;
  resetSelectedTraceID: () => void;
  selectedTraceID: string | null;
  setSelectedTraceID: (traceID: string | null) => void;
}

export const useDashboardAppStore = create<DashboardAppState>((set) => ({
  closeExportDialog: () => {
    set({ isExportDialogOpen: false });
  },
  incrementRefreshToken: () => {
    set((state) => ({ refreshToken: state.refreshToken + 1 }));
  },
  isExportDialogOpen: false,
  openExportDialog: () => {
    set({ isExportDialogOpen: true });
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
