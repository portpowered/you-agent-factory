import { useCallback, useEffect, useMemo } from "react";
import { create } from "zustand";

import type { AgentBentoLayoutItem } from "../components/agent-bento";
import type { DashboardWidgetPickerWidgetType } from "../lib/dashboard-widget-picker";
import {
  addDashboardWidgetToLayout,
  removeDashboardWidgetFromLayout,
} from "./dashboardLayoutMutations";
import { mergeDashboardLayout } from "./dashboardLayoutPersistence";
import {
  DASHBOARD_LAYOUT_STORAGE_KEY,
  type DashboardLayoutScope,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
} from "./dashboardLayoutSchema";
import {
  readStoredDashboardLayout,
  writeStoredDashboardLayout,
} from "./storage/dashboardLayoutStorage";

export {
  createDashboardLayoutScope,
  createDashboardWidgetInstanceID,
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_LAYOUT_STORAGE_KEY,
  DASHBOARD_LAYOUT_STORAGE_KEY_PREFIX,
  DASHBOARD_LAYOUT_STORAGE_VERSION,
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
  getRenderableDashboardLayout,
} from "./dashboardLayoutSchema";

export interface UseDashboardLayoutResult {
  addDashboardWidget: (widgetType: DashboardWidgetPickerWidgetType) => void;
  dashboardLayout: AgentBentoLayoutItem[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
  removeDashboardWidget: (widgetInstanceID: string) => void;
}

export type { DashboardLayoutScope } from "./dashboardLayoutSchema";

interface DashboardLayoutStoreState {
  addDashboardWidget: (
    scope: DashboardLayoutScope | null | undefined,
    widgetType: DashboardWidgetPickerWidgetType,
  ) => void;
  layoutsByStorageKey: Record<string, AgentBentoLayoutItem[]>;
  loadDashboardLayout: (scope: DashboardLayoutScope | null | undefined) => void;
  persistDashboardLayout: (
    scope: DashboardLayoutScope | null | undefined,
    layout: AgentBentoLayoutItem[],
  ) => void;
  removeDashboardWidget: (
    scope: DashboardLayoutScope | null | undefined,
    widgetInstanceID: string,
  ) => void;
}

const useDashboardLayoutStore = create<DashboardLayoutStoreState>((set) => ({
  addDashboardWidget: (scope, widgetType) => {
    set((state) => {
      const storageKey = getStorageKey(scope);
      const currentLayout =
        state.layoutsByStorageKey[storageKey] ??
        readStoredDashboardLayout(scope);
      const nextLayout = addDashboardWidgetToLayout(currentLayout, widgetType);
      writeStoredDashboardLayout(nextLayout, scope);
      return {
        layoutsByStorageKey: {
          ...state.layoutsByStorageKey,
          [storageKey]: nextLayout,
        },
      };
    });
  },
  layoutsByStorageKey: {
    [getStorageKey(undefined)]: readStoredDashboardLayout(),
  },
  loadDashboardLayout: (scope) => {
    set((state) => {
      const storageKey = getStorageKey(scope);
      return {
        layoutsByStorageKey: {
          ...state.layoutsByStorageKey,
          [storageKey]: readStoredDashboardLayout(scope),
        },
      };
    });
  },
  persistDashboardLayout: (scope, layout) => {
    set((state) => {
      const storageKey = getStorageKey(scope);
      const currentLayout =
        state.layoutsByStorageKey[storageKey] ??
        readStoredDashboardLayout(scope);
      const nextLayout = mergeDashboardLayout(layout, currentLayout);
      writeStoredDashboardLayout(nextLayout, scope);
      return {
        layoutsByStorageKey: {
          ...state.layoutsByStorageKey,
          [storageKey]: nextLayout,
        },
      };
    });
  },
  removeDashboardWidget: (scope, widgetInstanceID) => {
    set((state) => {
      const storageKey = getStorageKey(scope);
      const currentLayout =
        state.layoutsByStorageKey[storageKey] ??
        readStoredDashboardLayout(scope);
      const nextLayout = removeDashboardWidgetFromLayout(
        currentLayout,
        widgetInstanceID,
      );
      writeStoredDashboardLayout(nextLayout, scope);
      return {
        layoutsByStorageKey: {
          ...state.layoutsByStorageKey,
          [storageKey]: nextLayout,
        },
      };
    });
  },
}));

export function useDashboardLayout(
  scope?: DashboardLayoutScope | null,
): UseDashboardLayoutResult {
  const factoryID = scope?.factoryID;
  const sessionID = scope?.sessionID;
  const normalizedScope = useMemo(
    () =>
      factoryID === undefined || sessionID === undefined
        ? null
        : { factoryID, sessionID },
    [factoryID, sessionID],
  );
  const storageKey = getStorageKey(normalizedScope);
  const loadDashboardLayout = useDashboardLayoutStore(
    (state) => state.loadDashboardLayout,
  );
  const dashboardLayout = useDashboardLayoutStore(
    (state) =>
      state.layoutsByStorageKey[storageKey] ?? DEFAULT_DASHBOARD_LAYOUT,
  );

  useEffect(() => {
    loadDashboardLayout(normalizedScope);
  }, [loadDashboardLayout, normalizedScope]);

  const addDashboardWidget = useCallback(
    (widgetType: DashboardWidgetPickerWidgetType) => {
      useDashboardLayoutStore
        .getState()
        .addDashboardWidget(normalizedScope, widgetType);
    },
    [normalizedScope],
  );
  const persistDashboardLayout = useCallback(
    (layout: AgentBentoLayoutItem[]) => {
      useDashboardLayoutStore
        .getState()
        .persistDashboardLayout(normalizedScope, layout);
    },
    [normalizedScope],
  );
  const removeDashboardWidget = useCallback(
    (widgetInstanceID: string) => {
      useDashboardLayoutStore
        .getState()
        .removeDashboardWidget(normalizedScope, widgetInstanceID);
    },
    [normalizedScope],
  );

  return {
    addDashboardWidget,
    dashboardLayout,
    persistDashboardLayout,
    removeDashboardWidget,
  };
}

export function reloadDashboardLayoutFromStorage(
  scope?: DashboardLayoutScope | null,
): void {
  useDashboardLayoutStore.getState().loadDashboardLayout(scope);
}

function getStorageKey(scope?: DashboardLayoutScope | null): string {
  return scope
    ? getDashboardLayoutStorageKey(scope)
    : DASHBOARD_LAYOUT_STORAGE_KEY;
}
