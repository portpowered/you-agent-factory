import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_PAGE_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "./components/ui/dashboard-typography";
import { useDashboardStreamStore } from "./features/dashboard/state";
import * as factoryPngImportModule from "./features/import/factory-png-import";
import {
  createFactoryImportValue,
  createFileDropTransfer,
  jsonResponse,
  registerAppDashboardTestLifecycle,
  renderApp,
  terminalSnapshot,
} from "./testing/app-shell-test-utils";
import { currentNamedFactoryExportResponse } from "./testing/app-shell-export-test-utils";

describe("App shell locale and toolbar flows", () => {
  registerAppDashboardTestLifecycle();

  it("switches the live dashboard to zh-CN across header, dialogs, and widgets while keeping data values stable", async () => {
    const file = new File(["png"], "factory-import.png", { type: "image/png" });
    const importValue = createFactoryImportValue();
    vi.spyOn(factoryPngImportModule, "readFactoryImportPng").mockResolvedValue({
      ok: true,
      value: importValue,
    });
    const { fetchMock } = renderApp({
      initialLocale: "en",
      snapshot: terminalSnapshot,
    });

    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const path =
        typeof input === "string"
          ? input
          : input instanceof URL
            ? `${input.pathname}${input.search}`
            : input.url;

      if (path === "/factory-sessions/~default/factory") {
        return jsonResponse(currentNamedFactoryExportResponse);
      }

      throw new Error(`unexpected fetch for ${path}`);
    });

    const englishToolbar = await screen.findByRole("region", {
      name: "dashboard summary",
    });
    const languageButton = within(englishToolbar).getByRole("button", {
      name: "Change language",
    });

    expect(
      within(englishToolbar).getByRole("button", { name: "Export PNG" }),
    ).toBeTruthy();
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Factory graph" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();

    fireEvent.click(languageButton);
    fireEvent.click(
      screen.getByRole("menuitemradio", {
        name: "简体中文",
      }),
    );

    const localizedToolbar = await screen.findByRole("region", {
      name: "仪表板概览",
    });

    expect(
      within(localizedToolbar).getByRole("button", { name: "切换语言" }),
    ).toBeTruthy();
    expect(
      within(localizedToolbar).getByRole("slider", { name: "时间线刻度" }),
    ).toBeTruthy();
    expect(screen.getByText("正在等待更多刻度")).toBeTruthy();
    expect(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "工作总计" })).toBeTruthy();
    expect(screen.getByLabelText("已完成：1")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "工厂图" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "工作图视口" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Done Story" })).toBeTruthy();

    fireEvent.drop(
      screen.getByRole("region", { name: "工作图视口" }),
      createFileDropTransfer([file]),
    );

    const importDialog = await screen.findByRole("dialog", {
      name: "检查工厂导入",
    });
    expect(importDialog.textContent).toContain("Dropped Factory");
    expect(importDialog.textContent).toContain("factory-import.png");
    expect(
      within(importDialog).getByRole("img", {
        name: "Dropped Factory 预览图",
      }),
    ).toBeTruthy();
    expect(
      within(importDialog).getByRole("button", { name: "启用工厂" }),
    ).toBeTruthy();
    expect(
      within(importDialog).getByRole("button", { name: "取消导入" }),
    ).toBeTruthy();

    fireEvent.click(
      within(importDialog).getByRole("button", { name: "取消导入" }),
    );

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "检查工厂导入" })).toBeNull();
    });

    fireEvent.click(
      within(localizedToolbar).getByRole("button", { name: "导出 PNG" }),
    );

    const exportDialog = await screen.findByRole("dialog", {
      name: "导出工厂",
    });
    await waitFor(() => {
      expect(
        within(exportDialog).getByDisplayValue("semantic-workflow"),
      ).toBeTruthy();
    });
    expect(within(exportDialog).getByLabelText("工厂名称")).toBeTruthy();
    expect(within(exportDialog).getByLabelText("封面图片")).toBeTruthy();

    fireEvent.click(within(exportDialog).getByRole("button", { name: "取消" }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "导出工厂" })).toBeNull();
    });
  });

  it("falls back to English when the configured app locale is unsupported", async () => {
    renderApp({
      locationSearch: "?locale=fr-CA",
      snapshot: terminalSnapshot,
    });

    expect(
      await screen.findByRole("region", { name: "dashboard summary" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Change language" }),
    ).toBeTruthy();
    expect(screen.getByText("Waiting for more ticks")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Work totals" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "切换语言" })).toBeNull();
  });

  it("applies the shared typography helpers to the dashboard toolbar summary shell", async () => {
    renderApp({ snapshot: terminalSnapshot });

    const heading = await screen.findByRole("heading", {
      name: "you-agent-factory",
    });
    const toolbar = screen.getByRole("region", { name: "dashboard summary" });
    const streamStatus = screen.getByRole("status", {
      name: "you-agent-factory event stream connecting",
    });
    const exportButton = screen.getByRole("button", { name: "Export PNG" });

    expect(heading.className).toContain(DASHBOARD_PAGE_HEADING_CLASS);
    expect(streamStatus.className).toContain(DASHBOARD_BODY_TEXT_CLASS);
    expect(streamStatus.className).toContain(DASHBOARD_SUPPORTING_LABELS_CLASS);
    expect(within(toolbar).queryByText("Factory state")).toBeNull();
    expect(within(toolbar).queryByText(terminalSnapshot.factory_state)).toBeNull();
    expect(within(toolbar).queryByText("Loading factory events...")).toBeNull();
    expect(within(toolbar).queryByText("Export PNG")).toBeNull();
    expect(exportButton.getAttribute("aria-haspopup")).toBe("dialog");
  });

  it("renders compact accessible stream status states without the retired toolbar labels", async () => {
    renderApp({ snapshot: terminalSnapshot });

    const toolbar = await screen.findByRole("region", {
      name: "dashboard summary",
    });

    expect(
      within(toolbar).getByRole("status", {
        name: "you-agent-factory event stream connecting",
      }),
    ).toBeTruthy();
    expect(within(toolbar).queryByText("Factory state")).toBeNull();
    expect(within(toolbar).queryByText("Stream")).toBeNull();
    expect(within(toolbar).queryByText("Loading factory events...")).toBeNull();
    expect(within(toolbar).queryByText("Export PNG")).toBeNull();

    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          message: "Factory event stream connected.",
          status: "live",
        },
      });
    });

    await waitFor(() => {
      expect(
        within(toolbar).getByRole("status", {
          name: "you-agent-factory event stream live",
        }),
      ).toBeTruthy();
    });
    expect(
      within(toolbar).queryByText("Factory event stream connected."),
    ).toBeNull();

    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          message:
            "Factory event stream disconnected. Showing last event state.",
          status: "offline",
        },
      });
    });

    await waitFor(() => {
      expect(
        within(toolbar).getByRole("status", {
          name: "you-agent-factory event stream offline",
        }),
      ).toBeTruthy();
    });
    expect(
      within(toolbar).queryByText(
        "Factory event stream disconnected. Showing last event state.",
      ),
    ).toBeNull();
  });
});
