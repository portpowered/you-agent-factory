import {
  getInlineWidgetPickerMessages,
  getInlineWidgetPickerOptions,
} from "./inline-widget-picker";

describe("getInlineWidgetPickerMessages", () => {
  it("returns the English catalog by default", () => {
    expect(getInlineWidgetPickerMessages()).toMatchObject({
      addedState: "Already on dashboard",
      closeLabel: "Close widget picker",
      description:
        "Choose from the dashboard widgets available for inline management.",
      dismissAction: "Close",
      duplicateBadge: "Duplicate allowed",
      enabledState: "Add widget",
      openAction: "Browse widgets",
      selectionHint: "Choose a widget to add it immediately to the grid.",
      title: "Add dashboard widget",
    });
    expect(getInlineWidgetPickerMessages().options["work-graph"]).toEqual({
      description:
        "Watch workflow activity and navigate the graph from the main dashboard board.",
      title: "Workflow activity",
    });
  });

  it("returns localized picker copy when available", () => {
    expect(getInlineWidgetPickerMessages("zh-CN")).toMatchObject({
      addedState: "已在仪表板中",
      closeLabel: "关闭小组件选择器",
      dismissAction: "关闭",
      duplicateBadge: "允许重复",
      openAction: "浏览小组件",
      title: "添加仪表板小组件",
    });
    expect(getInlineWidgetPickerMessages("zh-CN").options.trace.title).toBe(
      "追踪钻取",
    );
  });
});

describe("getInlineWidgetPickerOptions", () => {
  it("returns the allowed widget catalog in picker order", () => {
    expect(getInlineWidgetPickerOptions().map((option) => option.widgetType)).toEqual([
      "work-totals",
      "work-graph",
      "current-selection",
      "provider-session",
      "terminal-work",
      "work-outcome-chart",
      "submit-work",
      "trace",
    ]);
  });
});
