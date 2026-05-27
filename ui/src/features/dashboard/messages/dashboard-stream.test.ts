import { createDefaultDashboardStreamState } from "../state/dashboardStreamStore";
import { getDashboardStreamMessages } from "./dashboard-stream";

describe("dashboard stream messages", () => {
  it("resolves default and zh-CN loading copy", () => {
    expect(getDashboardStreamMessages("en").loadingFactoryEvents).toBe(
      "Loading factory events...",
    );
    expect(getDashboardStreamMessages("zh-CN").loadingFactoryEvents).toBe(
      "正在加载工厂事件...",
    );
    expect(createDefaultDashboardStreamState("zh-CN")).toEqual({
      status: "connecting",
      message: "正在加载工厂事件...",
    });
  });
});
