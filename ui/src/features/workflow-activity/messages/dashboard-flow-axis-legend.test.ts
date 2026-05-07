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

  it.each([
    [
      "en",
      "Graph legend",
      "Expand Graph legend",
      "Collapse Graph legend",
      "Queue legend icon",
    ],
    [
      "zh",
      "图表图例",
      "展开图表图例",
      "收起图表图例",
      "队列图例图标",
    ],
    [
      "ko",
      "그래프 범례",
      "그래프 범례 펼치기",
      "그래프 범례 접기",
      "대기열 범례 아이콘",
    ],
    [
      "ja",
      "グラフの凡例",
      "グラフの凡例 を開く",
      "グラフの凡例 を閉じる",
      "キュー の凡例アイコン",
    ],
  ] as const)(
    "resolves %s helper labels through the locale catalog",
    (locale, title, expectedExpandLabel, expectedCollapseLabel, expectedIconLabel) => {
      const messages = getDashboardFlowAxisLegendMessages(locale);

      expect(messages.expandToggleLabel(title)).toBe(expectedExpandLabel);
      expect(messages.collapseToggleLabel(title)).toBe(expectedCollapseLabel);
      expect(messages.iconLabel(messages.iconLabels.queue)).toBe(
        expectedIconLabel,
      );
    },
  );

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
