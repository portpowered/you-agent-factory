import {
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
} from "../hooks/dashboardLayoutSchema";
import {
  DUPLICATE_CAPABLE_DASHBOARD_WIDGET_TYPES,
  getDashboardWidgetPickerAvailability,
} from "./dashboard-widget-picker";

describe("getDashboardWidgetPickerAvailability", () => {
  it("keeps duplicate-capable widgets enabled after they already exist", () => {
    const availability = getDashboardWidgetPickerAvailability(
      DEFAULT_DASHBOARD_LAYOUT,
    );

    expect(DUPLICATE_CAPABLE_DASHBOARD_WIDGET_TYPES).toEqual([
      DASHBOARD_WIDGET_IDS.terminalWork,
      DASHBOARD_WIDGET_IDS.workGraph,
      DASHBOARD_WIDGET_IDS.workOutcomeChart,
      DASHBOARD_WIDGET_IDS.workTotals,
    ]);
    expect(
      availability.find(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.workOutcomeChart,
      ),
    ).toEqual({
      duplicateCapable: true,
      enabled: true,
      widgetType: DASHBOARD_WIDGET_IDS.workOutcomeChart,
    });
  });

  it("disables non-duplicate widgets once they already exist in the layout", () => {
    const availability = getDashboardWidgetPickerAvailability(
      DEFAULT_DASHBOARD_LAYOUT,
    );

    expect(
      availability.find(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.currentSelection,
      ),
    ).toEqual({
      duplicateCapable: false,
      enabled: false,
      widgetType: DASHBOARD_WIDGET_IDS.currentSelection,
    });
  });
});
