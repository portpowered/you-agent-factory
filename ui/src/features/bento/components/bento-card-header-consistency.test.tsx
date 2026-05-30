import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { DashboardWidgetFrame } from "../../../components/ui/widget-frame";
import { InlineAddWidgetCard } from "../../dashboard-add-card/components/inline-add-widget-card";
import { WorkTotalsCard } from "../../work-totals/components/work-totals-card";
import { AgentBentoCard, AgentBentoLayout } from "./agent-bento";

function expectBentoCardHeaderSemantics(
  card: HTMLElement,
  title: string,
  {
    compactChrome = false,
  }: { compactChrome?: boolean } = {},
) {
  const header = card.querySelector("header");
  expect(header).toBeTruthy();

  expect(
    within(card).getByRole("heading", { level: 3, name: title }),
  ).toBeTruthy();

  expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
  expect(header?.className).toContain("cursor-grab");
  expect(
    within(card).queryByRole("button", { name: `Move ${title}` }),
  ).toBeNull();
  expect(
    header?.contains(
      within(card).getByRole("heading", { level: 3, name: title }),
    ),
  ).toBe(true);

  if (compactChrome) {
    expect(header?.className).toContain("min-h-11");
    expect(header?.className).toContain("flex-wrap");
  } else {
    expect(header?.className).toContain("min-h-13");
    expect(header?.className).not.toContain("flex-wrap");
  }
}

describe("bento card header consistency", () => {
  it("keeps default-density header semantics on a representative dashboard widget", () => {
    render(
      <DashboardWidgetFrame title="Provider session" widgetId="provider-session">
        <p>Session body</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    expectBentoCardHeaderSemantics(card, "Provider session");
  });

  it("keeps compact-density header semantics on a representative dashboard widget", () => {
    render(
      <WorkTotalsCard
        completedCount={1}
        dispatchedCount={2}
        failedCount={0}
        inFlightDispatchCount={1}
      />,
    );

    const card = screen.getByRole("article", { name: "Work totals" });
    expectBentoCardHeaderSemantics(card, "Work totals", {
      compactChrome: true,
    });
  });

  it("keeps compact-density header semantics on AgentBentoCard", () => {
    render(
      <AgentBentoCard chromeDensity="compact" title="Factory graph">
        <p>Graph body</p>
      </AgentBentoCard>,
    );

    const card = screen.getByRole("article", { name: "Factory graph" });
    expectBentoCardHeaderSemantics(card, "Factory graph", {
      compactChrome: true,
    });
  });

  describe("inline add-widget on bento grid", () => {
    beforeEach(() => {
      Object.defineProperty(HTMLElement.prototype, "offsetParent", {
        configurable: true,
        get() {
          return this.parentElement ?? document.body;
        },
      });
    });

    it("exposes the same header semantics as other bento cards", () => {
      render(
        <AgentBentoLayout
          cards={[
            {
              id: "add-widget::inline-add",
              widgetType: "add-widget",
              children: <InlineAddWidgetCard pickerAvailability={[]} />,
            },
          ]}
          initialWidth={960}
          layout={[
            {
              h: 4,
              id: "add-widget::inline-add",
              widgetType: "add-widget",
              w: 4,
              x: 0,
              y: 0,
            },
          ]}
        />,
      );

      const card = screen.getByRole("article", { name: "Add widget" });
      expectBentoCardHeaderSemantics(card, "Add widget");
    });
  });
});
