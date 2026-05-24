import { create } from "zustand";

import type { AgentBentoLayoutItem } from "../../../components/ui";
import {
  mergeDashboardLayout,
  readStoredDashboardLayout,
  writeStoredDashboardLayout,
} from "./dashboardLayoutPersistence";

export {
  createDashboardWidgetInstanceID,
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  getRenderableDashboardLayout,
} from "./dashboardLayoutSchema";

export interface UseDashboardLayoutResult {
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
}

interface DashboardLayoutStoreState {
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
}

const useDashboardLayoutStore = create<DashboardLayoutStoreState>((set) => ({
  dashboardLayout: readStoredDashboardLayout(),
  persistDashboardLayout: (layout) => {
    set((state) => {
      const nextLayout = mergeDashboardLayout(layout, state.dashboardLayout);
      writeStoredDashboardLayout(nextLayout);
      return { dashboardLayout: nextLayout };
    });
  },
}));

export function useDashboardLayout(): UseDashboardLayoutResult {
  const dashboardLayout = useDashboardLayoutStore((state) => state.dashboardLayout);
  const persistDashboardLayout = useDashboardLayoutStore((state) => state.persistDashboardLayout);

  return { dashboardLayout, persistDashboardLayout };
}

export function reloadDashboardLayoutFromStorage(): void {
  useDashboardLayoutStore.setState({
    dashboardLayout: readStoredDashboardLayout(),
  });
}
