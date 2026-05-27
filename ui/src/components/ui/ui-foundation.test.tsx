import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { installDashboardBrowserTestShims } from "../dashboard/test-browser-shims";
import { DataTable } from "./data-table";
import { UIFoundationShowcase } from "./ui-foundation-showcase";

describe("UIFoundationShowcase", () => {
  const restoreBrowserShims = installDashboardBrowserTestShims();

  afterAll(() => {
    restoreBrowserShims();
  });

  it("renders the shared primitive baseline with interactive evidence", async () => {
    const user = userEvent.setup();

    render(<UIFoundationShowcase />);

    expect(screen.getByRole("button", { name: "Primary action" })).toBeTruthy();
    expect(
      screen
        .getByRole("button", { name: "Disabled action" })
        .hasAttribute("disabled"),
    ).toBe(true);
    expect(screen.getByRole("textbox", { name: "Request name" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "Request text" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Work type" })).toBeTruthy();
    expect(
      screen.getByRole("img", { name: "Primitive chart showcase" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("table", {
        name: "Primitive table foundation for trace and detail surfaces.",
      }),
    ).toBeTruthy();
    expect(
      screen.getByRole("table", { name: "Primitive data table showcase" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Primitive calendar showcase")).toBeTruthy();
    expect(screen.getByText("Sidebar panel")).toBeTruthy();
    expect(screen.getByText("Detail panel")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Open dialog" }));
    const dialog = await screen.findByRole("dialog", {
      name: "Export factory",
    });
    expect(
      within(dialog).getByRole("button", { name: "Confirm export" }),
    ).toBeTruthy();
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    const collapseButton = screen.getByRole("button", { name: "Collapse" });
    await user.click(collapseButton);
    expect(
      screen
        .getByRole("button", { name: "Expand" })
        .getAttribute("aria-expanded"),
    ).toBe("false");
  });

  it("renders shared primitive copy from a non-default locale catalog", async () => {
    const user = userEvent.setup();

    render(<UIFoundationShowcase locale="zh-CN" />);

    expect(
      screen.getByRole("heading", { name: "共享 UI 基础组件" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "主要操作" })).toBeTruthy();
    expect(screen.getByRole("textbox", { name: "请求名称" })).toBeTruthy();
    expect(screen.getByRole("img", { name: "基础图表展示" })).toBeTruthy();
    expect(screen.getByLabelText("基础日历展示")).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "打开对话框" }));
    const dialog = await screen.findByRole("dialog", { name: "导出工厂" });
    expect(
      within(dialog).getByRole("button", { name: "确认导出" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("button", { name: "关闭对话框" }),
    ).toBeTruthy();
  });

  it("localizes the default empty state for shared data tables", () => {
    render(
      <DataTable
        ariaLabel="empty table"
        columns={[{ cell: () => null, header: "Column", id: "column" }]}
        data={[]}
        getRowKey={() => "unused"}
        locale="zh-CN"
      />,
    );

    expect(screen.getByText("没有可用行。")).toBeTruthy();
  });
});
