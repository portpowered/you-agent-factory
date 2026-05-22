import { act, renderHook } from "@testing-library/react";

import {
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  reloadDashboardLayoutFromStorage,
  useDashboardLayout,
} from "./useDashboardLayout";

describe("useDashboardLayout", () => {
  beforeEach(() => {
    window.localStorage.clear();
    act(() => {
      reloadDashboardLayoutFromStorage();
    });
  });

  it("includes the provider-session widget in the default dashboard layout", () => {
    const { result } = renderHook(() => useDashboardLayout());

    expect(result.current.dashboardLayout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
    expect(result.current.dashboardLayout).toContainEqual({
      h: 5,
      id: DASHBOARD_WIDGET_IDS.providerSession,
      minH: 4,
      minW: 3,
      w: 4,
      x: 4,
      y: 12,
    });
  });

  it("migrates saved layouts created before the provider-session widget existed", () => {
    const legacyLayout = DEFAULT_DASHBOARD_LAYOUT.filter(
      (item) => item.id !== DASHBOARD_WIDGET_IDS.providerSession,
    ).map((item) =>
      item.id === DASHBOARD_WIDGET_IDS.currentSelection
        ? { ...item, h: 7, x: 1, y: 14 }
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
      DASHBOARD_WIDGET_IDS.providerSession,
    );
    expect(
      migratedLayout.find((item) => item.id === DASHBOARD_WIDGET_IDS.currentSelection),
    ).toMatchObject({
      h: 7,
      x: 1,
      y: 14,
    });
    expect(
      migratedLayout.find((item) => item.id === DASHBOARD_WIDGET_IDS.providerSession),
    ).toEqual(
      expect.objectContaining(
        DEFAULT_DASHBOARD_LAYOUT.find(
          (item) => item.id === DASHBOARD_WIDGET_IDS.providerSession,
        ),
      ),
    );
  });
});
