import { SUPPORTED_LOCALES } from "../../../i18n";
import {
  dashboardFlowAxisLegendMessagesByLocale,
  getDashboardFlowAxisLegendMessages,
} from "./dashboard-flow-axis-legend";

describe("getDashboardFlowAxisLegendMessages", () => {
  it("supports the expected workflow activity legend locales", () => {
    expect(Object.keys(dashboardFlowAxisLegendMessagesByLocale).sort()).toEqual(
      [...SUPPORTED_LOCALES].sort(),
    );
  });

  it.each([
    ["en", "Graph legend"],
    ["zh", "图表图例"],
    ["ko", "그래프 범례"],
    ["ja", "グラフの凡例"],
  ] as const)("resolves %s legend copy", (locale, expectedTitle) => {
    expect(getDashboardFlowAxisLegendMessages(locale).title).toBe(
      expectedTitle,
    );
  });

  it("falls back to the default locale when the locale is missing or unsupported", () => {
    const defaultMessages = getDashboardFlowAxisLegendMessages("en");

    expect(getDashboardFlowAxisLegendMessages(undefined).title).toBe(
      defaultMessages.title,
    );
    expect(getDashboardFlowAxisLegendMessages("fr").title).toBe(
      defaultMessages.title,
    );
  });

  it("keeps accessible label helpers and icon labels available through the resolved locale catalog", () => {
    const messages = getDashboardFlowAxisLegendMessages("zh");

    expect(messages.minimizedLabel).toBe("图例");
    expect(messages.edgeLabels.failurePath).toBe("失败路径");
    expect(messages.iconLabels.workstation).toBe("标准工作站");
    expect(messages.expandToggleLabel(messages.title)).toBe("展开图表图例");
    expect(messages.iconLabel(messages.iconLabels.queue)).toBe("队列图例图标");
  });
});
