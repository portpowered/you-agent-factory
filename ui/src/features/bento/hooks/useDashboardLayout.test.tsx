import { act, renderHook } from "@testing-library/react";

import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  DASHBOARD_LAYOUT_STORAGE_KEY,
  reloadDashboardLayoutFromStorage,
  useDashboardLayout,
} from "./useDashboardLayout";

function resetDashboardLayoutStorage() {
  window.localStorage.clear();
  act(() => {
    reloadDashboardLayoutFromStorage();
  });
}

describe("useDashboardLayout default layout", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("includes the provider-session widget in the default dashboard layout", () => {
    const { result } = renderHook(() => useDashboardLayout());

    expect(result.current.dashboardLayout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.current.dashboardLayout).toContainEqual({
      h: 5,
      id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.providerSession,
      minH: 3,
      minW: 2,
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
});

describe("useDashboardLayout core migrations", () => {
  beforeEach(resetDashboardLayoutStorage);

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
});

describe("useDashboardLayout persisted layout merging", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("reloads stored layouts with the current runtime resize bounds instead of stale persisted minimums", () => {
    window.localStorage.setItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
      JSON.stringify([
        {
          h: 5,
          id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
          minH: 4,
          minW: 3,
          w: 4,
          widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
          x: 0,
          y: 10,
        },
        {
          h: 11,
          id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace,
          minH: 7,
          minW: 5,
          w: 8,
          widgetType: DASHBOARD_WIDGET_IDS.trace,
          x: 0,
          y: 15,
        },
      ]),
    );

    act(() => {
      reloadDashboardLayoutFromStorage();
    });

    const { result } = renderHook(() => useDashboardLayout());

    expect(
      result.current.dashboardLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      ),
    ).toMatchObject({
      minH: 3,
      minW: 2,
    });
    expect(
      result.current.dashboardLayout.find((item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace),
    ).toMatchObject({
      minH: 6,
      minW: 4,
    });
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

describe("useDashboardLayout migration-specific layout compaction", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("migrates legacy compacted widget positions and restores the provider-session primary card", () => {
    window.localStorage.setItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
      JSON.stringify([
        { h: 10, id: DASHBOARD_WIDGET_IDS.workGraph, w: 12, x: 0, y: 2 },
        { h: 5, id: DASHBOARD_WIDGET_IDS.currentSelection, w: 4, x: 0, y: 12 },
        { h: 5, id: DASHBOARD_WIDGET_IDS.terminalWork, w: 4, x: 4, y: 12 },
        { h: 6, id: DASHBOARD_WIDGET_IDS.workOutcomeChart, w: 4, x: 8, y: 17 },
        { h: 6, id: DASHBOARD_WIDGET_IDS.submitWork, w: 4, x: 8, y: 23 },
        { h: 9, id: DASHBOARD_WIDGET_IDS.trace, w: 8, x: 0, y: 18 },
      ]),
    );

    act(() => {
      reloadDashboardLayoutFromStorage();
    });

    const { result } = renderHook(() => useDashboardLayout());
    const migratedLayout = result.current.dashboardLayout;

    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph,
      ),
    ).toMatchObject({ h: 8, y: 2 });
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      ),
    ).toMatchObject({ y: 10 });
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.terminalWork,
      ),
    ).toMatchObject({ x: 8, y: 10 });
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workOutcomeChart,
      ),
    ).toMatchObject({ y: 15 });
    expect(
      migratedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.submitWork,
      ),
    ).toMatchObject({ y: 21 });
    expect(
      migratedLayout.find((item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace),
    ).toMatchObject({ y: 15 });
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

  it("widens legacy trace cards and drops stale max-width constraints during reload migration", () => {
    window.localStorage.setItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
      JSON.stringify([
        {
          h: 7,
          id: DASHBOARD_WIDGET_IDS.trace,
          maxW: 8,
          w: 4,
          x: 0,
          y: 18,
        },
        {
          h: 9,
          id: "trace::instance-1",
          maxW: 9,
          widgetType: DASHBOARD_WIDGET_IDS.trace,
          w: 8,
          x: 0,
          y: 15,
        },
      ]),
    );

    act(() => {
      reloadDashboardLayoutFromStorage();
    });

    const { result } = renderHook(() => useDashboardLayout());
    const primaryTrace = result.current.dashboardLayout.find(
      (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace,
    );
    const duplicateTrace = result.current.dashboardLayout.find(
      (item) => item.id === "trace::instance-1",
    );

    expect(primaryTrace).toMatchObject({
      h: 9,
      id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.trace,
      minH: 6,
      minW: 4,
      w: 8,
      widgetType: DASHBOARD_WIDGET_IDS.trace,
      x: 0,
      y: 15,
    });
    expect(duplicateTrace).toMatchObject({
      id: "trace::instance-1",
      widgetType: DASHBOARD_WIDGET_IDS.trace,
      w: 8,
      x: 0,
      y: 15,
    });
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

  it("adds duplicate-capable widgets with stable instance ids and keeps one add-widget card", () => {
    const { result } = renderHook(() => useDashboardLayout());

    act(() => {
      result.current.addDashboardWidget(DASHBOARD_WIDGET_IDS.workOutcomeChart);
    });

    const nextLayout = result.current.dashboardLayout;
    const workOutcomeCards = nextLayout.filter(
      (item) => item.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart,
    );
    const addWidgetCards = nextLayout.filter(
      (item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget,
    );

    expect(workOutcomeCards).toHaveLength(2);
    expect(workOutcomeCards.map((item) => item.id)).toEqual(
      expect.arrayContaining([
        DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workOutcomeChart,
        "work-outcome-chart::instance-1",
      ]),
    );
    expect(addWidgetCards).toHaveLength(1);
    expect(
      new Set(
        workOutcomeCards.map((item) => `${item.x}:${item.y}:${item.w}:${item.h}`),
      ).size,
    ).toBe(2);
  });

  it("does not add a second copy of a non-duplicate widget type", () => {
    const { result } = renderHook(() => useDashboardLayout());
    const initialLayout = result.current.dashboardLayout;

    act(() => {
      result.current.addDashboardWidget(DASHBOARD_WIDGET_IDS.currentSelection);
    });

    expect(result.current.dashboardLayout).toEqual(initialLayout);
    expect(
      result.current.dashboardLayout.filter(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.currentSelection,
      ),
    ).toHaveLength(1);
  });

  it("removes only the targeted widget instance and keeps the inline add-widget card", () => {
    const { result } = renderHook(() => useDashboardLayout());

    act(() => {
      result.current.addDashboardWidget(DASHBOARD_WIDGET_IDS.workOutcomeChart);
    });

    const duplicateInstance = result.current.dashboardLayout.find(
      (item) => item.id === "work-outcome-chart::instance-1",
    );

    expect(duplicateInstance).toBeDefined();

    act(() => {
      result.current.removeDashboardWidget("work-outcome-chart::instance-1");
    });

    expect(
      result.current.dashboardLayout.filter(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart,
      ),
    ).toHaveLength(1);
    expect(
      result.current.dashboardLayout.find(
        (item) => item.id === "work-outcome-chart::instance-1",
      ),
    ).toBeUndefined();
    expect(
      result.current.dashboardLayout.find(
        (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
      ),
    ).toBeDefined();
  });

});

describe("useDashboardLayout reload persistence", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("restores added, removed, moved, and resized widget instances from local storage on reload", () => {
    const { result } = renderHook(() => useDashboardLayout());

    act(() => {
      result.current.addDashboardWidget(DASHBOARD_WIDGET_IDS.workOutcomeChart);
    });

    act(() => {
      result.current.persistDashboardLayout(
        result.current.dashboardLayout.map((item) => {
          if (item.id === "work-outcome-chart::instance-1") {
            return { ...item, h: 8, w: 6, x: 2, y: 28 };
          }

          if (item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID) {
            return { ...item, h: 3, w: 3, x: 9, y: 28 };
          }

          return item;
        }),
      );
      result.current.removeDashboardWidget(
        DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      );
      reloadDashboardLayoutFromStorage();
    });

    const reloadedLayout = result.current.dashboardLayout;

    expect(
      reloadedLayout.find(
        (item) => item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      ),
    ).toMatchObject({
      hidden: true,
      id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.currentSelection,
      widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
    });
    expect(
      reloadedLayout.filter(
        (item) =>
          item.widgetType === DASHBOARD_WIDGET_IDS.currentSelection && !item.hidden,
      ),
    ).toHaveLength(0);
    expect(
      reloadedLayout.find((item) => item.id === "work-outcome-chart::instance-1"),
    ).toMatchObject({
      h: 8,
      id: "work-outcome-chart::instance-1",
      w: 6,
      widgetType: DASHBOARD_WIDGET_IDS.workOutcomeChart,
      x: 2,
      y: 28,
    });
    expect(
      reloadedLayout.filter((item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget),
    ).toHaveLength(1);
    expect(
      reloadedLayout.find((item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID),
    ).toMatchObject({
      h: 3,
      id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
      w: 3,
      widgetType: DASHBOARD_WIDGET_IDS.addWidget,
      x: 9,
      y: 28,
    });
  });
});
