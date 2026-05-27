import { create } from "zustand";

import type { DashboardStreamState } from "../../../api/dashboard/types";
import { getDashboardStreamMessages } from "../messages/dashboard-stream";

interface DashboardStreamStoreState {
  resetStreamState: (locale?: string | null) => void;
  setStreamState: (streamState: DashboardStreamState) => void;
  streamState: DashboardStreamState;
}

export function createDefaultDashboardStreamState(
  locale?: string | null,
): DashboardStreamState {
  return {
    status: "connecting",
    message: getDashboardStreamMessages(locale).loadingFactoryEvents,
  };
}

export const useDashboardStreamStore = create<DashboardStreamStoreState>((set) => ({
  resetStreamState: (locale) => {
    set({ streamState: createDefaultDashboardStreamState(locale) });
  },
  setStreamState: (streamState) => {
    set({ streamState });
  },
  streamState: createDefaultDashboardStreamState(),
}));
