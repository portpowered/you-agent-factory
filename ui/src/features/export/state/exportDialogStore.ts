import { create } from "zustand";

interface ExportDialogState {
  closeExportDialog: () => void;
  isExportDialogOpen: boolean;
  openExportDialog: () => void;
}

export const useExportDialogStore = create<ExportDialogState>((set) => ({
  closeExportDialog: () => {
    set({ isExportDialogOpen: false });
  },
  isExportDialogOpen: false,
  openExportDialog: () => {
    set({ isExportDialogOpen: true });
  },
}));

