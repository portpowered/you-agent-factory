import {
  getWorkflowActivityShellMessages,
  workflowActivityShellMessagesByLocale,
} from "./activity-shell";

describe("getWorkflowActivityShellMessages", () => {
  it("returns the localized zh-CN workflow activity shell copy", () => {
    const messages = getWorkflowActivityShellMessages("zh-CN");

    expect(messages.title).toBe("当前活动");
    expect(messages.viewportLabel).toBe("工作图视口");
    expect(messages.selectStateLabel("完成")).toBe("选择 完成 状态");
    expect(messages.selectWorkstationLabel("审查")).toBe("选择 审查 工作站");
    expect(messages.selectExhaustionRuleLabel("审查")).toBe(
      "选择 审查 枯竭规则",
    );
  });

  it("falls back to English for unsupported locales and keeps the locale catalog exported", () => {
    const messages = getWorkflowActivityShellMessages("fr-FR");

    expect(messages.title).toBe("Current activity");
    expect(messages.widgetTitle).toBe("Factory graph");
    expect(messages.selectStateLabel("Done")).toBe("Select Done state");
    expect(workflowActivityShellMessagesByLocale.en.emptyTitle).toBe(
      "No workflow topology loaded",
    );
  });
});
