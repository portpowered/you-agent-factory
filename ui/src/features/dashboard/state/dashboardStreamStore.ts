import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";

const DEFAULT_STREAM_STATE: DashboardStreamState = {
  status: "connecting",
  message: "Loading factory events...",
};

interface DashboardStreamStoreState {
  resetStreamState: () => void;
  setStreamState: (streamState: DashboardStreamState) => void;
  streamState: DashboardStreamState;
}

export function createDefaultDashboardStreamState(): DashboardStreamState {
  return { ...DEFAULT_STREAM_STATE };
}

export const useDashboardStreamStore = create<DashboardStreamStoreState>((set) => ({
  resetStreamState: () => {
    set({ streamState: createDefaultDashboardStreamState() });
  },
  setStreamState: (streamState) => {
    set({ streamState });
  },
  streamState: createDefaultDashboardStreamState(),
}));
