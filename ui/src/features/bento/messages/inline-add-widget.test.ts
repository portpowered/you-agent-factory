import { getInlineAddWidgetMessages } from "./inline-add-widget";

describe("getInlineAddWidgetMessages", () => {
  it("returns the English catalog by default", () => {
    expect(getInlineAddWidgetMessages()).toEqual({
      actionLabel: "Browse widgets",
      actionUnavailableLabel: "No widgets available",
      title: "Add widget",
    });
  });

  it("falls back to English for unsupported locales", () => {
    expect(getInlineAddWidgetMessages("fr")).toEqual(
      getInlineAddWidgetMessages("en"),
    );
  });

  it("returns the localized catalog when available", () => {
    expect(getInlineAddWidgetMessages("zh-CN")).toEqual({
      actionLabel: "浏览小组件",
      actionUnavailableLabel: "没有可用小组件",
      title: "添加小组件",
    });
  });
});
