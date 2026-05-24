import { act, renderHook } from "@testing-library/react";

import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  reloadDashboardLayoutFromStorage,
  useDashboardLayout,
} from "./useDashboardLayout";

function resetDashboardLayoutStorage() {
  window.localStorage.clear();
  act(() => {
    reloadDashboardLayoutFromStorage();
  });
}

describe("useDashboardLayout defaults and migrations", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("includes the provider-session widget in the default dashboard layout", () => {
    const { result } = renderHook(() => useDashboardLayout());

    expect(result.current.dashboardLayout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.current.dashboardLayout).toContainEqual({
      h: 5,
      id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
      minH: 4,
      minW: 3,
      w: 4,
      widgetType: DASHBOARD_WIDGET_IDS.providerSession,
      x: 4,
      y: 10,
    });
    expect(result.current.dashboardLayout).toContainEqual(
      expect.objectContaining({
        id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
        widgetType: DASHBOARD_WIDGET_IDS.addWidget,
      }),
    );
  });

  it("migrates saved layouts created before the provider-session widget existed", () => {
    const legacyLayout = DEFAULT_DASHBOARD_LAYOUT.filter(
      (item) => item.id !== DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
    ).map((item) =>
      item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection
        ? { h: 7, id: DASHBOARD_WIDGET_IDS.currentSelection, w: item.w, x: 1, y: 14 }
        : item,
    );
    window.localStorage.setItem(
      "agent-factory.dashboard.layout.v2",
      JSON.stringify(legacyLayout),
    );

    act(() => {
      reloadDashboardLayoutFromStorage();
    });

    const { result } = renderHook(() => useDashboardLayout());
    const migratedLayout = result.current.dashboardLayout;

    expect(migratedLayout.map((item) => item.id)).toContain(
      DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
    );
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      ),
    ).toMatchObject({
      h: 7,
      widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
      x: 1,
      y: 14,
    });
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
      ),
    ).toEqual(
      expect.objectContaining(
        DEFAULT_DASHBOARD_LAYOUT.find(
          (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
        ),
      ),
    );
  });

  it("preserves the inline add-widget layout entry when persisting only rendered widgets", () => {
    const { result } = renderHook(() => useDashboardLayout());
    const renderedLayout = result.current.dashboardLayout.filter(
      (item) => item.widgetType !== DASHBOARD_WIDGET_IDS.addWidget,
    );

    act(() => {
      result.current.persistDashboardLayout(
        renderedLayout.map((item) =>
          item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph
            ? { ...item, h: 9, y: 3 }
            : item,
        ),
      );
    });

    const storedLayout = JSON.parse(
      window.localStorage.getItem("agent-factory.dashboard.layout.v2") ?? "[]",
    ) as Array<{ id: string; h: number; y: number; widgetType: string }>;

    expect(storedLayout).toContainEqual(
      expect.objectContaining({
        id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
        widgetType: DASHBOARD_WIDGET_IDS.addWidget,
      }),
    );
    expect(storedLayout).toContainEqual(
      expect.objectContaining({
        h: 9,
        id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph,
        widgetType: DASHBOARD_WIDGET_IDS.workGraph,
        y: 3,
      }),
    );
  });
});

describe("useDashboardLayout inline add-widget persistence", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("persists inline add-widget repositioning alongside normal dashboard cards", () => {
    const { result } = renderHook(() => useDashboardLayout());

    act(() => {
      result.current.persistDashboardLayout(
        result.current.dashboardLayout.map((item) =>
          item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID
            ? { ...item, h: 5, w: 3, x: 5, y: 24 }
            : item,
        ),
      );
    });

    const storedLayout = JSON.parse(
      window.localStorage.getItem("agent-factory.dashboard.layout.v2") ?? "[]",
    ) as Array<{
      h: number;
      id: string;
      w: number;
      widgetType: string;
      x: number;
      y: number;
    }>;

    expect(
      storedLayout.filter((item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget),
    ).toHaveLength(1);
    expect(
      storedLayout.find((item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID),
    ).toMatchObject({
      h: 5,
      id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
      w: 3,
      widgetType: DASHBOARD_WIDGET_IDS.addWidget,
      x: 5,
      y: 24,
    });
  });
});
