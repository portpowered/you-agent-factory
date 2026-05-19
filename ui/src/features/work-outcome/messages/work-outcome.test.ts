import { getWorkOutcomeMessages } from "./work-outcome";

describe("getWorkOutcomeMessages", () => {
  it("returns English chart and trend labels by default", () => {
    const messages = getWorkOutcomeMessages();

    expect(messages.chart.ariaLabel("15m")).toBe("Work outcome chart for 15m");
    expect(messages.chart.tickLabel(20)).toBe("Tick 20");
    expect(messages.chart.resetZoomAction).toBe("Reset zoom");
    expect(messages.trends.failureChartAriaLabel("15m")).toBe(
      "Failed work trend for 15m",
    );
    expect(messages.trends.reworkChartAriaLabel("Story A")).toBe(
      "Retry and rework trend for Story A",
    );
    expect(messages.trends.timingChartAriaLabel("Story A")).toBe(
      "Timing trend for Story A",
    );
    expect(messages.trends.rangeOptionLabel("custom", "Last hour")).toBe(
      "Last hour",
    );
  });

  it("returns zh-CN chart and trend labels for the localized dashboard path", () => {
    const messages = getWorkOutcomeMessages("zh-CN");

    expect(messages.chart.ariaLabel("15m")).toBe("15m 的工作结果图表");
    expect(messages.chart.tickLabel(20)).toBe("刻度 20");
    expect(messages.chart.resetZoomLabel).toBe("重置工作结果图表缩放");
    expect(messages.trends.failureChartAriaLabel("15m")).toBe("15m 的失败工作趋势");
    expect(messages.trends.reworkChartAriaLabel("故事 A")).toBe(
      "故事 A 的重试与返工趋势",
    );
    expect(messages.trends.timingChartAriaLabel("故事 A")).toBe("故事 A 的时序趋势");
    expect(messages.trends.rangeOptionLabel("session", "Session")).toBe("会话");
    expect(messages.trends.rangeOptionLabel("custom", "Session")).toBe("Session");
  });
});
