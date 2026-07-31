import "../../../testing/vitest-dom-capabilities.setup";

import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import {
  DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
} from "./dashboardLayoutSchema";
import { useDashboardLayout } from "./useDashboardLayout";

function resetDashboardLayoutStorage() {
  window.localStorage.removeItem("agent-factory.dashboard.layout.v2");
}

describe("useDashboardLayout factory-session widget placement", () => {
  beforeEach(resetDashboardLayoutStorage);

  it("adds the hidden primary factory-session widget and repositions the add-widget card", () => {
    const { result } = renderHook(() => useDashboardLayout());

    act(() => {
      result.current.addDashboardWidget(DASHBOARD_WIDGET_IDS.factorySession);
    });

    const factorySessionCard = result.current.dashboardLayout.find(
      (item) =>
        item.id === DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.factorySession,
    );
    const addWidgetCard = result.current.dashboardLayout.find(
      (item) => item.id === DASHBOARD_INLINE_ADD_WIDGET_INSTANCE_ID,
    );

    expect(factorySessionCard).toMatchObject({
      hidden: undefined,
      h: 4,
      w: 4,
      widgetType: DASHBOARD_WIDGET_IDS.factorySession,
      x: 8,
      y: 27,
    });
    expect(addWidgetCard).toMatchObject({
      x: 0,
      y: 28,
    });
  });
});
