import { getInlineAddWidgetMessages } from "./inline-add-widget";

describe("getInlineAddWidgetMessages", () => {
  it("returns the English catalog by default", () => {
    expect(getInlineAddWidgetMessages()).toEqual({
      badge: "Dashboard",
      body: "Add a widget to this dashboard grid.",
      hint: "Browse available dashboard widgets without leaving this grid.",
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
      hint: "无需离开此网格即可浏览可用的仪表板小组件。",
      iconTitle: "添加小组件图标",
      title: "添加小组件",
    });
  });
});
