// @vitest-environment happy-dom

import { describe, expect, it } from "vitest";

import { renderPackageComponent, screen, within } from "../testing/render";
import { WidgetFrame } from "./widget-frame";
import {
  WidgetDetailCopy,
  WidgetEmptyState,
  WidgetSubtitle,
} from "./widget-frame-content";

describe("WidgetFrame", () => {
  it("renders the shared widget frame contract with host-provided content", () => {
    renderPackageComponent(
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

    expect(subtitle.className).toContain("text-display-small");
    expect(bodyCopy.className).toContain("text-body-medium");
    expect(emptyHeading.parentElement?.className).toContain("border-dashed");
  });

  it("supports the wide widget frame layout variant", () => {
    renderPackageComponent(
      <WidgetFrame title="Trend card" wide>
        <p>Trend content</p>
      </WidgetFrame>,
    );

    expect(
      screen.getByRole("article", { name: "Trend card" }).className,
    ).toContain("min-h-72");
  });

  it("passes body props through to the widget frame body", () => {
    renderPackageComponent(
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
    renderPackageComponent(
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
    renderPackageComponent(
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
    renderPackageComponent(
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
