import { render, screen, within } from "@testing-library/react";

import {
  WIDGET_FRAME_BODY_TEXT_CLASS,
  WIDGET_FRAME_SECTION_HEADING_CLASS,
  WIDGET_FRAME_SUBTITLE_CLASS,
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
  WidgetFrame,
  WidgetSubtitle,
} from "@you-agent-factory/components/recipes";

describe("WidgetEmptyState", () => {
  it("renders compact dashboard empty states through the component contract", () => {
    render(
      <WidgetEmptyState compact>
        <h3>No trace selected</h3>
      </WidgetEmptyState>,
    );

    const emptyHeading = screen.getByRole("heading", {
      name: "No trace selected",
    });
    expect(emptyHeading.parentElement?.className).toContain("min-h-0");
    expect(emptyHeading.parentElement?.className).toContain(
      "bg-surface-container-low",
    );
  });

  it("renders empty-state title and body copy through shared typography roles", () => {
    render(
      <WidgetEmptyState>
        <WidgetEmptyStateTitle as="h2">No chart data</WidgetEmptyStateTitle>
        <WidgetEmptyStateText>
          Run the factory to populate this trend.
        </WidgetEmptyStateText>
      </WidgetEmptyState>,
    );

    const title = screen.getByRole("heading", {
      level: 2,
      name: "No chart data",
    });
    const body = screen.getByText("Run the factory to populate this trend.");

    expect(title.className).toContain(WIDGET_FRAME_SECTION_HEADING_CLASS);
    expect(body.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
    expect(body.className).toContain("m-0");
  });
});

describe("WidgetSubtitle", () => {
  it("supports subtitle text on non-paragraph semantic elements", () => {
    render(
      <dl>
        <WidgetSubtitle as="dd">42 completed</WidgetSubtitle>
      </dl>,
    );

    const value = screen.getByText("42 completed");

    expect(value.tagName).toBe("DD");
    expect(value.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
  });
});

describe("WidgetFrame", () => {
  it("renders the package widget frame shell with host-provided content", () => {
    render(
      <WidgetFrame title="Example widget">
        <WidgetSubtitle>Queue a new request</WidgetSubtitle>
        <WidgetDetailCopy>
          Submissions stay inside the shared layout frame.
        </WidgetDetailCopy>
        <WidgetEmptyState>
          <h3>No active submission</h3>
        </WidgetEmptyState>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Example widget" });
    const title = within(card).getByRole("heading", {
      level: 3,
      name: "Example widget",
    });

    expect(title).toBeTruthy();
    expect(card.querySelectorAll("header")).toHaveLength(1);
    expect(card.className).toContain("min-w-0");
    expect(card.className).toContain("shadow-af-card");
    expect(card.className).toContain("[&_dl]:grid");
    expect(card.className).toContain("border-outline");
    expect(card.className).toContain("bg-surface-container-high");

    const subtitle = within(card).getByText("Queue a new request");
    const bodyCopy = within(card).getByText(
      "Submissions stay inside the shared layout frame.",
    );
    const emptyHeading = within(card).getByRole("heading", {
      name: "No active submission",
    });

    expect(subtitle.className).toContain(WIDGET_FRAME_SUBTITLE_CLASS);
    expect(bodyCopy.className).toContain(WIDGET_FRAME_BODY_TEXT_CLASS);
    expect(emptyHeading.parentElement?.className).toContain("border-dashed");
  });

  it("supports the wide widget frame layout variant", () => {
    render(
      <WidgetFrame title="Trend card" wide>
        <p>Trend content</p>
      </WidgetFrame>,
    );

    expect(
      screen.getByRole("article", { name: "Trend card" }).className,
    ).toContain("min-h-72");
  });

  it("passes body props through to the widget frame body", () => {
    render(
      <WidgetFrame
        bodyClassName="custom-widget-body"
        bodyProps={{ "data-widget-body": "trace-card" }}
        title="Trace"
      >
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const body = screen
      .getByText("Trace content")
      .closest("[data-widget-body]");

    expect(body?.getAttribute("data-widget-body")).toBe("trace-card");
    expect(body?.className).toContain("custom-widget-body");
  });

  it("uses scrollable bodies by default", () => {
    render(
      <WidgetFrame title="Trace">
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });
    const body = screen.getByText("Trace content").parentElement;

    expect(card.className).toContain("overflow-hidden");
    expect(body?.className).toContain("overflow-auto");
  });

  it("supports opting out of the internal scroll body", () => {
    render(
      <WidgetFrame bodyScroll={false} title="Trace">
        <p>Trace content</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Trace" });
    const body = screen.getByText("Trace content").parentElement;

    expect(card.className).not.toContain("overflow-hidden");
    expect(body?.className).not.toContain("overflow-auto");
  });

  it("routes header actions through the widget frame header", () => {
    render(
      <WidgetFrame
        headerAction={<button type="button">Remove card</button>}
        title="Provider session"
      >
        <p>Session details</p>
      </WidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Provider session" });
    const header = card.querySelector("header");
    const toolsRegion = header?.lastElementChild;

    expect(
      within(card).getByRole("heading", {
        level: 3,
        name: "Provider session",
      }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Remove card" }),
    ).toBeTruthy();
    expect(
      toolsRegion?.contains(
        screen.getByRole("button", { name: "Remove card" }),
      ),
    ).toBe(true);
  });
});
