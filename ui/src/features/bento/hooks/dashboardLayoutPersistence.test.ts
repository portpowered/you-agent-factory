import { sanitizeDashboardLayout } from "./dashboardLayoutPersistence";
import {
  DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS,
  DASHBOARD_WIDGET_IDS,
  DEFAULT_DASHBOARD_LAYOUT,
  getDefaultWidgetLayoutByType,
} from "./dashboardLayoutSchema";

function layoutItem(
  widgetType: string,
  id: string,
  overrides: Partial<ReturnType<typeof getDefaultWidgetLayoutByType>> = {},
) {
  const defaultItem = getDefaultWidgetLayoutByType(widgetType);
  if (!defaultItem) {
    throw new Error(`missing default layout for ${widgetType}`);
  }
  return { ...defaultItem, id, widgetType, ...overrides };
}

function visibleItemsDoNotOverlap(
  layout: readonly {
    h: number;
    hidden?: boolean;
    w: number;
    x: number;
    y: number;
  }[],
) {
  const visibleLayout = layout.filter((item) => !item.hidden);
  return visibleLayout.every((item, index) =>
    visibleLayout
      .slice(index + 1)
      .every(
        (otherItem) =>
          item.x + item.w <= otherItem.x ||
          otherItem.x + otherItem.w <= item.x ||
          item.y + item.h <= otherItem.y ||
          otherItem.y + otherItem.h <= item.y,
      ),
  );
}

describe("sanitizeDashboardLayout", () => {
  it("repairs unsafe items, ids, sizes, and coordinates without exposing raw ids", () => {
    const result = sanitizeDashboardLayout([
      layoutItem(DASHBOARD_WIDGET_IDS.workOutcomeChart, "<unsafe-id>", {
        h: 0,
        w: 999,
        x: -4,
        y: -2,
      }),
      null as never,
      { id: "unsupported", widgetType: "unsupported" } as never,
    ]);

    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual(
      expect.arrayContaining([
        "invalid-id",
        "invalid-item",
        "invalid-size",
        "out-of-bounds",
      ]),
    );
    expect(result.layout.every((item) => !/[<>]/u.test(item.id))).toBe(true);
    expect(result.layout.every((item) => item.w > 0 && item.h > 0)).toBe(true);
    expect(
      result.layout.every(
        (item) => item.x >= 0 && item.x + item.w <= 12 && item.y >= 0,
      ),
    ).toBe(true);
  });

  it("reports and repairs a missing card identity", () => {
    const { id: _missingID, ...itemWithoutID } = layoutItem(
      DASHBOARD_WIDGET_IDS.workTotals,
      "work-totals::instance-1",
    );

    const result = sanitizeDashboardLayout([itemWithoutID]);

    expect(result.diagnostics).toContainEqual({
      code: "invalid-id",
      count: 1,
      severity: "repair",
    });
    expect(result.layout).toContainEqual(
      expect.objectContaining({
        id: "work-totals::primary",
        widgetType: DASHBOARD_WIDGET_IDS.workTotals,
      }),
    );
  });

  it("repairs duplicate identities and singleton violations deterministically", () => {
    const duplicateID = "work-outcome-chart::instance-1";
    const result = sanitizeDashboardLayout([
      layoutItem(DASHBOARD_WIDGET_IDS.workOutcomeChart, duplicateID),
      layoutItem(DASHBOARD_WIDGET_IDS.workOutcomeChart, duplicateID),
      layoutItem(
        DASHBOARD_WIDGET_IDS.providerSession,
        "provider-session::instance-1",
      ),
      layoutItem(
        DASHBOARD_WIDGET_IDS.providerSession,
        "provider-session::instance-2",
      ),
      layoutItem(DASHBOARD_WIDGET_IDS.workGraph, "work-graph::instance-1"),
    ]);

    expect(result.diagnostics.map((diagnostic) => diagnostic.code)).toEqual(
      expect.arrayContaining(["duplicate-id", "singleton-violation"]),
    );
    expect(new Set(result.layout.map((item) => item.id)).size).toBe(
      result.layout.length,
    );
    expect(
      result.layout.filter(
        (item) =>
          item.widgetType === DASHBOARD_WIDGET_IDS.providerSession &&
          !item.hidden,
      ),
    ).toHaveLength(1);
    expect(
      result.layout.filter(
        (item) => item.widgetType === DASHBOARD_WIDGET_IDS.workGraph,
      ),
    ).toHaveLength(1);
  });

  it("repositions colliding cards into a non-overlapping in-bounds layout", () => {
    const result = sanitizeDashboardLayout([
      layoutItem(
        DASHBOARD_WIDGET_IDS.workOutcomeChart,
        "work-outcome-chart::instance-1",
        { x: 0, y: 0 },
      ),
      layoutItem(
        DASHBOARD_WIDGET_IDS.workOutcomeChart,
        "work-outcome-chart::instance-2",
        { x: 0, y: 0 },
      ),
    ]);

    expect(result.diagnostics).toContainEqual(
      expect.objectContaining({ code: "collision" }),
    );
    expect(visibleItemsDoNotOverlap(result.layout)).toBe(true);
  });

  it("leaves a canonical layout without diagnostics when the saved layout is safe", () => {
    const result = sanitizeDashboardLayout(DEFAULT_DASHBOARD_LAYOUT);

    expect(result.diagnostics).toEqual([]);
    expect(result.layout).toEqual(DEFAULT_DASHBOARD_LAYOUT);
  });
});

describe("sanitizeDashboardLayout graph replacement", () => {
  it("keeps a visible graph replacement instead of restoring the default graph identity", () => {
    const replacementGraph = layoutItem(
      DASHBOARD_WIDGET_IDS.workGraph,
      "work-graph::instance-1",
    );
    const result = sanitizeDashboardLayout(
      DEFAULT_DASHBOARD_LAYOUT.map((item) =>
        item.widgetType === DASHBOARD_WIDGET_IDS.workGraph
          ? replacementGraph
          : item,
      ),
    );

    expect(result.layout).toContainEqual(replacementGraph);
    expect(result.layout).not.toContainEqual(
      expect.objectContaining({
        id: DASHBOARD_PRIMARY_WIDGET_INSTANCE_IDS.workGraph,
      }),
    );
    expect(result.diagnostics).toEqual([]);
  });
});
