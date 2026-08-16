import { getAgentBentoMessages } from "./agent-bento";

describe("getAgentBentoMessages", () => {
  it("returns the default English dashboard board and remove labels", () => {
    const messages = getAgentBentoMessages();

    expect(messages.boardLabel).toBe("you-agent-factory bento board");
    expect(messages.cardErrorTitle).toBe("Dashboard card unavailable");
    expect(messages.retryCard).toBe("Retry card");
    expect(messages.removeWidgetLabel("Work totals")).toBe(
      "Remove Work totals widget from dashboard",
    );
  });

  it("returns Chinese remove labels when requested", () => {
    const messages = getAgentBentoMessages("zh-CN");

    expect(messages.boardLabel).toBe("you-agent-factory Bento 看板");
    expect(messages.cardErrorTitle).toBe("仪表板卡片不可用");
    expect(messages.retryCard).toBe("重试卡片");
    expect(messages.removeWidgetLabel("工作总览")).toBe(
      "从仪表板移除 工作总览 小组件",
    );
  });
});
