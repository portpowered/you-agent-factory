import { useCallback, useEffect, useMemo } from "react";
import { create } from "zustand";

import type { AgentBentoLayoutItem } from "../components/agent-bento";
import type { DashboardWidgetPickerWidgetType } from "../lib/dashboard-widget-picker";
import {
  addDashboardWidgetToLayout,
  allocateDashboardWidgetInstance,
  removeDashboardWidgetFromLayout,
} from "./dashboardLayoutMutations";
import { mergeDashboardLayout } from "./dashboardLayoutPersistence";
import {
  DASHBOARD_LAYOUT_STORAGE_KEY,
  type DashboardLayoutDiagnostic,
  type DashboardLayoutInstanceHighWaterMarks,
  type DashboardLayoutScope,
  DEFAULT_DASHBOARD_LAYOUT,
  getDashboardLayoutStorageKey,
} from "./dashboardLayoutSchema";
import {
  readStoredDashboardLayoutResult,
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
  dashboardLayoutDiagnostics: DashboardLayoutDiagnostic[];
  persistDashboardLayout: (layout: AgentBentoLayoutItem[]) => void;
  removeDashboardWidget: (widgetInstanceID: string) => void;
}

export type {
  DashboardLayoutDiagnostic,
  DashboardLayoutDiagnosticCode,
  DashboardLayoutInstanceHighWaterMarks,
  DashboardLayoutScope,
} from "./dashboardLayoutSchema";

interface DashboardLayoutStoreState {
  addDashboardWidget: (
    scope: DashboardLayoutScope | null | undefined,
    widgetType: DashboardWidgetPickerWidgetType,
  ) => void;
  diagnosticsByStorageKey: Record<string, DashboardLayoutDiagnostic[]>;
  instanceHighWaterMarksByStorageKey: Record<
    string,
    DashboardLayoutInstanceHighWaterMarks
  >;
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

const initialDashboardLayoutStorageResult = readStoredDashboardLayoutResult();

interface DashboardLayoutStateAtScope {
  currentHighWaterMarks: DashboardLayoutInstanceHighWaterMarks;
  currentLayout: AgentBentoLayoutItem[];
  storageKey: string;
  storedResult: ReturnType<typeof readStoredDashboardLayoutResult> | null;
}

type DashboardLayoutStoreMutationState = Pick<
  DashboardLayoutStoreState,
  | "diagnosticsByStorageKey"
  | "instanceHighWaterMarksByStorageKey"
  | "layoutsByStorageKey"
>;

function getDashboardLayoutStateAtScope(
  state: DashboardLayoutStoreState,
  scope: DashboardLayoutScope | null | undefined,
): DashboardLayoutStateAtScope {
  const storageKey = getStorageKey(scope);
  const storedResult = state.layoutsByStorageKey[storageKey]
    ? null
    : readStoredDashboardLayoutResult(scope);
  return {
    currentHighWaterMarks:
      state.instanceHighWaterMarksByStorageKey[storageKey] ??
      storedResult?.instanceHighWaterMarks ??
      {},
    currentLayout:
      state.layoutsByStorageKey[storageKey] ??
      storedResult?.layout ??
      DEFAULT_DASHBOARD_LAYOUT,
    storageKey,
    storedResult,
  };
}

function persistDashboardLayoutState(
  state: DashboardLayoutStoreState,
  scope: DashboardLayoutScope | null | undefined,
  layoutState: DashboardLayoutStateAtScope,
  nextLayout: AgentBentoLayoutItem[],
  instanceHighWaterMarks: DashboardLayoutInstanceHighWaterMarks,
): DashboardLayoutStoreMutationState {
  const writeResult = writeStoredDashboardLayout(
    nextLayout,
    scope,
    instanceHighWaterMarks,
  );
  return {
    diagnosticsByStorageKey: {
      ...state.diagnosticsByStorageKey,
      [layoutState.storageKey]: combineDashboardLayoutDiagnostics(
        state.diagnosticsByStorageKey[layoutState.storageKey] ??
          layoutState.storedResult?.diagnostics ??
          [],
        writeResult.diagnostics,
      ),
    },
    instanceHighWaterMarksByStorageKey: {
      ...state.instanceHighWaterMarksByStorageKey,
      [layoutState.storageKey]: writeResult.instanceHighWaterMarks,
    },
    layoutsByStorageKey: {
      ...state.layoutsByStorageKey,
      [layoutState.storageKey]: nextLayout,
    },
  };
}

const useDashboardLayoutStore = create<DashboardLayoutStoreState>((set) => ({
  addDashboardWidget: (scope, widgetType) => {
    set((state) => {
      const layoutState = getDashboardLayoutStateAtScope(state, scope);
      const allocation = allocateDashboardWidgetInstance(
        layoutState.currentLayout,
        widgetType,
        layoutState.currentHighWaterMarks,
      );
      if (!allocation.instanceID) {
        return state;
      }

      const addedLayout = addDashboardWidgetToLayout(
        layoutState.currentLayout,
        widgetType,
        allocation.instanceID,
      );
      if (addedLayout === layoutState.currentLayout) {
        return state;
      }

      const nextLayout = mergeDashboardLayout(
        addedLayout,
        layoutState.currentLayout,
      );
      return persistDashboardLayoutState(
        state,
        scope,
        layoutState,
        nextLayout,
        allocation.instanceHighWaterMarks,
      );
    });
  },
  layoutsByStorageKey: {
    [getStorageKey(undefined)]: initialDashboardLayoutStorageResult.layout,
  },
  instanceHighWaterMarksByStorageKey: {
    [getStorageKey(undefined)]:
      initialDashboardLayoutStorageResult.instanceHighWaterMarks,
  },
  diagnosticsByStorageKey: {
    [getStorageKey(undefined)]: initialDashboardLayoutStorageResult.diagnostics,
  },
  loadDashboardLayout: (scope) => {
    set((state) => {
      const storageKey = getStorageKey(scope);
      const storedResult = readStoredDashboardLayoutResult(scope);
      return {
        diagnosticsByStorageKey: {
          ...state.diagnosticsByStorageKey,
          [storageKey]: storedResult.diagnostics,
        },
        layoutsByStorageKey: {
          ...state.layoutsByStorageKey,
          [storageKey]: storedResult.layout,
        },
        instanceHighWaterMarksByStorageKey: {
          ...state.instanceHighWaterMarksByStorageKey,
          [storageKey]: storedResult.instanceHighWaterMarks,
        },
      };
    });
  },
  persistDashboardLayout: (scope, layout) => {
    set((state) => {
      const layoutState = getDashboardLayoutStateAtScope(state, scope);
      const nextLayout = mergeDashboardLayout(
        layout,
        layoutState.currentLayout,
      );
      return persistDashboardLayoutState(
        state,
        scope,
        layoutState,
        nextLayout,
        layoutState.currentHighWaterMarks,
      );
    });
  },
  removeDashboardWidget: (scope, widgetInstanceID) => {
    set((state) => {
      const layoutState = getDashboardLayoutStateAtScope(state, scope);
      const nextLayout = removeDashboardWidgetFromLayout(
        layoutState.currentLayout,
        widgetInstanceID,
      );
      return persistDashboardLayoutState(
        state,
        scope,
        layoutState,
        nextLayout,
        layoutState.currentHighWaterMarks,
      );
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
  const dashboardLayoutDiagnostics = useDashboardLayoutStore(
    (state) => state.diagnosticsByStorageKey[storageKey] ?? [],
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
    dashboardLayoutDiagnostics,
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

function combineDashboardLayoutDiagnostics(
  ...diagnosticLists: readonly DashboardLayoutDiagnostic[][]
): DashboardLayoutDiagnostic[] {
  const diagnosticsByCode = new Map<
    DashboardLayoutDiagnostic["code"],
    DashboardLayoutDiagnostic
  >();
  for (const diagnostics of diagnosticLists) {
    for (const diagnostic of diagnostics) {
      const previous = diagnosticsByCode.get(diagnostic.code);
      diagnosticsByCode.set(diagnostic.code, {
        code: diagnostic.code,
        count: (previous?.count ?? 0) + diagnostic.count,
        severity:
          previous?.severity === "error" || diagnostic.severity === "error"
            ? "error"
            : "repair",
      });
    }
  }
  return [...diagnosticsByCode.values()];
}
