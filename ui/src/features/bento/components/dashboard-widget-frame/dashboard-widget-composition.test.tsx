import { bunVi as vi } from "../../../../testing/bun/vi-compat";
import "../../../../testing/vitest-dom-capabilities.setup";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { FactorySessionEventReplayDisclosure } from "../../../factory-session-detail/components/event-replay/factory-session-event-replay-disclosure";
import { ProviderSessionWidget } from "../../../provider-session-detail/components/provider-session-widget";
import { DEFAULT_DASHBOARD_LAYOUT } from "../../hooks/dashboardLayoutSchema";
import {
  canAddDashboardWidgetType,
  getDashboardWidgetPickerAvailability,
} from "../../lib/dashboard-widget-picker";
import { AgentBentoCard, toCompactGridLayout } from "../agent-bento";
import { DashboardWidgetFrame } from "./dashboard-widget-frame";

const SELECTED_SESSION = {
  dispatchID: "dispatch-review-active",
  id: "sess_composition",
  kind: "session_id",
  provider: "codex",
} as const;

function renderWithQueryClient(view: ReactNode) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>{view}</QueryClientProvider>,
  );
}

describe("dashboard widget composition ownership", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("routes dashboard widget frames through feature-owned bento card chrome", () => {
    render(
      <DashboardWidgetFrame
        title="Provider session"
        widgetId="provider-session"
      >
        <p>Session body</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    const header = card.querySelector("header");

    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(
      card.querySelector("[data-radix-scroll-area-viewport]"),
    ).toBeTruthy();
    expect(within(card).getByText("Session body")).toBeTruthy();
  });

  it("keeps widget picker and layout selection rules in dashboard bento feature code", () => {
    const availability = getDashboardWidgetPickerAvailability(
      DEFAULT_DASHBOARD_LAYOUT,
    );

    expect(
      availability.some(
        (entry) =>
          entry.widgetType === "work-graph" && entry.duplicateCapable === true,
      ),
    ).toBe(true);
    expect(
      canAddDashboardWidgetType(DEFAULT_DASHBOARD_LAYOUT, "work-graph"),
    ).toBe(true);
    expect(
      canAddDashboardWidgetType(DEFAULT_DASHBOARD_LAYOUT, "provider-session"),
    ).toBe(false);
  });

  it("renders loading and empty states through dashboard-owned widget composition", async () => {
    vi.mocked(globalThis.fetch).mockReturnValue(new Promise(() => undefined));

    renderWithQueryClient(
      <ProviderSessionWidget selectedProviderSession={SELECTED_SESSION} />,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    expect(within(card).getByRole("status").textContent).toContain(
      "Loading session details...",
    );

    vi.unstubAllGlobals();
    vi.stubGlobal("fetch", vi.fn());

    renderWithQueryClient(
      <ProviderSessionWidget selectedProviderSession={null} />,
    );

    expect(
      screen.getByText(
        "Select a provider session from work-item or workstation history to inspect session details.",
      ),
    ).toBeTruthy();
  });

  it("keeps collapsed and expanded disclosure behavior in dashboard feature widgets", async () => {
    vi.mocked(globalThis.fetch).mockResolvedValue(
      new Response(JSON.stringify({ events: [] }), {
        headers: { "Content-Type": "application/json" },
        status: 200,
      }),
    );

    renderWithQueryClient(
      <FactorySessionEventReplayDisclosure sessionID="sess-replay-composition" />,
    );

    const user = userEvent.setup();
    const replayTrigger = screen.getByRole("button", {
      name: "Expand Factory Event replay",
    });

    expect(replayTrigger.getAttribute("aria-expanded")).toBe("false");

    await user.click(replayTrigger);

    await waitFor(() => {
      expect(replayTrigger.getAttribute("aria-expanded")).toBe("true");
    });
  });

  it("stacks bento cards into a single full-width column for compact responsive layouts", () => {
    const compactLayout = toCompactGridLayout([
      { h: 4, i: "alpha", w: 4, x: 0, y: 0 },
      { h: 3, i: "beta", w: 6, x: 4, y: 0 },
      { h: 2, i: "gamma", w: 3, x: 8, y: 2 },
    ]);

    expect(compactLayout).toEqual([
      expect.objectContaining({ i: "alpha", w: 12, x: 0, y: 0 }),
      expect.objectContaining({ i: "beta", w: 12, x: 0, y: 4 }),
      expect.objectContaining({ i: "gamma", w: 12, x: 0, y: 7 }),
    ]);
  });

  it("keeps dashboard copy and action placement on feature-owned bento cards", () => {
    render(
      <AgentBentoCard
        headerAction={<button type="button">Remove card</button>}
        title="Work totals"
      >
        <p>Totals body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Work totals" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;

    expect(
      within(card).getByRole("button", { name: "Remove card" }),
    ).toBeTruthy();
    expect(
      toolsRegion?.contains(
        screen.getByRole("button", { name: "Remove card" }),
      ),
    ).toBe(true);
    expect(within(card).getByText("Totals body")).toBeTruthy();
  });
});
