import { getInlineAddWidgetMessages } from "./inline-add-widget";

describe("getInlineAddWidgetMessages", () => {
  it("returns the English catalog by default", () => {
    expect(getInlineAddWidgetMessages()).toEqual({
      badge: "Dashboard",
      body: "Add a widget to this dashboard grid.",
      hint: "The widget picker opens here in the next step.",
      iconTitle: "Add widget icon",
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
      badge: "仪表板",
      body: "将小组件添加到此仪表板网格。",
      hint: "下一步会在这里打开小组件选择器。",
      iconTitle: "添加小组件图标",
      title: "添加小组件",
    });
  });
});
