import { describe, expect, it } from "vitest";

import { getFactorySessionWidgetMessages } from "./factory-session-widget";

describe("getFactorySessionWidgetMessages", () => {
  it("resolves supported locale copy", () => {
    expect(getFactorySessionWidgetMessages("en")).toEqual({
      emptyState:
        "Select a live factory session to inspect orchestrator runtime.",
      title: "Factory session",
    });
    expect(getFactorySessionWidgetMessages("zh-CN")).toEqual({
      emptyState: "选择一个实时工厂会话来查看编排器运行时。",
      title: "工厂会话",
    });
  });

  it("falls back to English for unsupported locales", () => {
    expect(getFactorySessionWidgetMessages("fr")).toEqual(
      getFactorySessionWidgetMessages("en"),
    );
  });
});
