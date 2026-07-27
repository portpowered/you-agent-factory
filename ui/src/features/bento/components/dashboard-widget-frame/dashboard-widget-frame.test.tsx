import "../../../../testing/vitest-dom-capabilities.setup";

import { render, screen, within } from "@testing-library/react";

import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WIDGET_FRAME_SUPPORTING_LABELS_CLASS,
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";
import { DashboardWidgetFrame } from "./dashboard-widget-frame";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: widget-frame chrome cases share one rendering contract and fixture vocabulary.
describe("DashboardWidgetFrame chrome", () => {
  it("renders the shared widget frame contract with dashboard copy styles intact", () => {
    render(
      <DashboardWidgetFrame title="Submit work" widgetId="submit-work">
        <WidgetSubtitle>Queue a new request</WidgetSubtitle>
        <WidgetDetailCopy>
          Submissions stay inside the shared layout frame.
        </WidgetDetailCopy>
        <WidgetEmptyState>
          <h3>No active submission</h3>
        </WidgetEmptyState>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Submit work" });
    const title = within(card).getByRole("heading", {
      level: 3,
      name: "Submit work",
    });
    const header = card.querySelector("header");

    expect(title).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(
      within(card).queryByRole("button", { name: "Move Submit work" }),
    ).toBeNull();
    expect(card.querySelectorAll("header")).toHaveLength(1);

    const subtitle = within(card).getByText("Queue a new request");
    const bodyCopy = within(card).getByText(
      "Submissions stay inside the shared layout frame.",
    );
    const emptyHeading = within(card).getByRole("heading", {
      name: "No active submission",
    });

    expect(card.className).toContain("min-w-0");
    expect(card.dataset.dashboardPanelShell).toBe("grid-card");
    expect(card.className).toContain("shadow-af-card");
    expect(card.className).toContain("[&_dl]:grid");
    expect(card.className).toContain(WIDGET_FRAME_SUPPORTING_LABELS_CLASS);
    expect(card.className).toContain("border-outline");
    expect(card.className).toContain("bg-surface-container-high");
    expect(subtitle.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
    expect(bodyCopy.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
    expect(emptyHeading.parentElement?.className).toContain("border-dashed");
    expect(emptyHeading.parentElement?.className).toContain(
      "border-outline-variant",
    );
    expect(emptyHeading.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
  });

  it("supports the wide dashboard widget frame layout variant", () => {
    render(
      <DashboardWidgetFrame title="Trend card" widgetId="trend-card" wide>
        <p>Trend content</p>
      </DashboardWidgetFrame>,
    );

    expect(
      screen.getByRole("article", { name: "Trend card" }).className,
    ).toContain("min-h-72");
  });

  it("passes body props through to the shared bento card body", () => {
    render(
      <DashboardWidgetFrame
        bodyClassName="custom-widget-body"
        bodyProps={{ "data-widget-body": "trace-card" }}
        title="Trace"
        widgetId="trace"
      >
        <p>Trace content</p>
      </DashboardWidgetFrame>,
    );

    const body = screen
      .getByText("Trace content")
      .closest("[data-widget-body]");

    expect(body?.getAttribute("data-widget-body")).toBe("trace-card");
    expect(body?.className).toContain("custom-widget-body");
  });

  it("uses scrollable bodies by default through the shared bento card", () => {
    render(
      <DashboardWidgetFrame title="Trace" widgetId="trace">
        <p>Trace content</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });

    expect(
      card.querySelector("[data-radix-scroll-area-viewport]"),
    ).toBeTruthy();
    expect(card.className).toContain("overflow-hidden");
  });

  it("supports opting out of the internal scroll body when a card needs page-flow layout", () => {
    render(
      <DashboardWidgetFrame bodyScroll={false} title="Trace" widgetId="trace">
        <p>Trace content</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });

    expect(card.querySelector("[data-radix-scroll-area-viewport]")).toBeNull();
    expect(card.className).not.toContain("overflow-hidden");
  });

  it("routes header actions through AgentBentoCard without a custom header slot", () => {
    render(
      <DashboardWidgetFrame
        headerAction={<button type="button">Remove card</button>}
        title="Provider session"
        widgetId="provider-session"
      >
        <p>Session details</p>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;

    expect(
      within(card).getByRole("heading", { level: 3, name: "Provider session" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Remove card" }),
    ).toBeTruthy();
    expect(header?.getAttribute("data-bento-drag-handle")).toBe("true");
    expect(header?.className).toContain("cursor-grab");
    expect(
      within(card).queryByRole("button", { name: "Move Provider session" }),
    ).toBeNull();
    expect(
      toolsRegion?.contains(
        screen.getByRole("button", { name: "Remove card" }),
      ),
    ).toBe(true);
  });
});
