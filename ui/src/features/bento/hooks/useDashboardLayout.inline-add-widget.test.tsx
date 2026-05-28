import { act, renderHook } from "@testing-library/react";

import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_LAYOUT_STORAGE_KEY,
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

describe("useDashboardLayout inline add-widget reload persistence", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("deduplicates persisted inline add-widget entries back to one canonical item on reload", () => {
    window.localStorage.setItem(
      DASHBOARD_LAYOUT_STORAGE_KEY,
      JSON.stringify([
        ...DEFAULT_DASHBOARD_LAYOUT,
        {
          h: 5,
          id: "add-widget::duplicate",
          minH: 3,
          minW: 3,
          w: 5,
          widgetType: DASHBOARD_WIDGET_IDS.addWidget,
          x: 4,
          y: 31,
        },
        {
          h: 4,
          id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
          minH: 3,
          minW: 3,
          w: 3,
          widgetType: DASHBOARD_WIDGET_IDS.addWidget,
          x: 9,
          y: 27,
        },
      ]),
    );

    act(() => {
      reloadDashboardLayoutFromStorage();
    });

    const { result } = renderHook(() => useDashboardLayout());
    const addWidgetItems = result.current.dashboardLayout.filter(
      (item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget,
    );

    expect(addWidgetItems).toHaveLength(1);
    expect(addWidgetItems[0]).toMatchObject({
      h: 4,
      id: DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
      w: 3,
      widgetType: DASHBOARD_WIDGET_IDS.addWidget,
      x: 9,
      y: 27,
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
      window.localStorage.getItem(DASHBOARD_LAYOUT_STORAGE_KEY) ?? "[]",
    ) as Array<{
      h: number;
      id: string;
      w: number;
      widgetType: string;
      x: number;
      y: number;
    }>;

    expect(
      storedLayout.filter(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.addWidget,
      ),
    ).toHaveLength(1);
    expect(
      storedLayout.find(
        (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
      ),
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
