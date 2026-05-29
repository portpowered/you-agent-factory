import { create } from "zustand";

import type { AgentBentoLayoutItem } from "../components/agent-bento";
import type { DashboardWidgetPickerWidgetType } from "../lib/dashboard-widget-picker";
import {
  addDashboardWidgetToLayout,
  removeDashboardWidgetFromLayout,
} from "./dashboardLayoutMutations";
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
  addDashboardWidget: (widgetType: DashboardWidgetPickerWidgetType) => void;
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
  removeDashboardWidget: (widgetInstanceID: string) => void;
}

interface DashboardLayoutStoreState {
  addDashboardWidget: (widgetType: DashboardWidgetPickerWidgetType) => void;
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
  removeDashboardWidget: (widgetInstanceID: string) => void;
}

const useDashboardLayoutStore = create<DashboardLayoutStoreState>((set) => ({
  addDashboardWidget: (widgetType) => {
    set((state) => {
      const nextLayout = addDashboardWidgetToLayout(
        state.dashboardLayout,
        widgetType,
      );
      writeStoredDashboardLayout(nextLayout);
      return { dashboardLayout: nextLayout };
    });
  },
  dashboardLayout: readStoredDashboardLayout(),
  persistDashboardLayout: (layout) => {
    set((state) => {
      const nextLayout = mergeDashboardLayout(layout, state.dashboardLayout);
      writeStoredDashboardLayout(nextLayout);
      return { dashboardLayout: nextLayout };
    });
  },
  removeDashboardWidget: (widgetInstanceID) => {
    set((state) => {
      const nextLayout = removeDashboardWidgetFromLayout(
        state.dashboardLayout,
        widgetInstanceID,
      );
      writeStoredDashboardLayout(nextLayout);
      return { dashboardLayout: nextLayout };
    });
  },
}));

export function useDashboardLayout(): UseDashboardLayoutResult {
  const addDashboardWidget = useDashboardLayoutStore(
    (state) => state.addDashboardWidget,
  );
  const dashboardLayout = useDashboardLayoutStore(
    (state) => state.dashboardLayout,
  );
  const persistDashboardLayout = useDashboardLayoutStore(
    (state) => state.persistDashboardLayout,
  );
  const removeDashboardWidget = useDashboardLayoutStore(
    (state) => state.removeDashboardWidget,
  );

  return {
    addDashboardWidget,
    dashboardLayout,
    persistDashboardLayout,
    removeDashboardWidget,
  };
}

export function reloadDashboardLayoutFromStorage(): void {
  useDashboardLayoutStore.setState({
    dashboardLayout: readStoredDashboardLayout(),
  });
}
