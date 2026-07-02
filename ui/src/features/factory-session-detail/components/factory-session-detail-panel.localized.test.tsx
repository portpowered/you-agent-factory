import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { FactorySessionDetailPanel } from "./factory-session-detail-panel";
import { renderWithQueryClient } from "./test-support/factory-session-detail-panel.test-helpers";

describe("FactorySessionDetailPanel localized states", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("shows a not-found state when the durable JavaScript session is missing", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Factory session not found.",
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel sessionID="dur-sess-js-missing-001" />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "Loading factory session runtime…",
    );
    expect(screen.getByText("dur-sess-js-missing-001")).toBeTruthy();

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "This factory session is no longer available.",
      );
    });
    expect(screen.queryByText("Runtime")).toBeNull();
  });

  it("renders zh-CN durable JavaScript loading and missing states", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(
        JSON.stringify({
          code: "NOT_FOUND",
          message: "Factory session not found.",
        }),
        {
          headers: { "Content-Type": "application/json" },
          status: 404,
          statusText: "Not Found",
        },
      ),
    );

    renderWithQueryClient(
      <FactorySessionDetailPanel
        locale="zh-CN"
        sessionID="dur-sess-js-missing-001"
      />,
    );

    expect(screen.getByRole("status").textContent).toContain(
      "正在加载工厂会话运行时…",
    );

    await waitFor(() => {
      expect(screen.getByRole("status").textContent).toContain(
        "此工厂会话已不可用。",
      );
    });
  });
});
