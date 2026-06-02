import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach } from "vitest";

import { installDashboardBrowserTestShims } from "../dashboard/test-browser-shims";
import { ScrollArea } from "./scroll-area";

function getScrollViewport(container: HTMLElement): HTMLElement {
  const viewport = container.querySelector("[data-radix-scroll-area-viewport]");
  if (!(viewport instanceof HTMLElement)) {
    throw new Error("Expected a Radix scroll-area viewport");
  }
  return viewport;
}

function constrainScrollViewport(
  viewport: HTMLElement,
  clientHeightPx: number,
  scrollHeightPx: number,
): HTMLElement {
  Object.defineProperty(viewport, "clientHeight", {
    configurable: true,
    value: clientHeightPx,
  });
  Object.defineProperty(viewport, "scrollHeight", {
    configurable: true,
    value: scrollHeightPx,
  });
  viewport.scrollTop = 0;
  return viewport;
}

describe("ScrollArea", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders a scroll viewport with semantic scrollbar styling", () => {
    const { container } = render(
      <ScrollArea className="h-32" data-testid="scroll-root" type="always">
        <div style={{ height: "240px" }}>Overflowing content</div>
      </ScrollArea>,
    );

    const root = screen.getByTestId("scroll-root");
    const viewport = getScrollViewport(container);
    const scrollbar = container.querySelector('[data-orientation="vertical"]');

    expect(root.className).toContain("overflow-hidden");
    expect(viewport.className).toContain("[scrollbar-width:none]");
    expect(viewport.className).toContain("[&::-webkit-scrollbar]:hidden");
    expect(scrollbar).toBeTruthy();
    expect(scrollbar?.className).toContain("w-2.5");
    expect(scrollbar?.className).toContain("bg-transparent");
  });

  it("forwards viewport className and data attributes to the scroll viewport", () => {
    const { container } = render(
      <ScrollArea
        viewportClassName="min-h-0 flex-1"
        viewportProps={{ "data-trace-card-scroll": "" }}
      >
        <p>Trace card body</p>
      </ScrollArea>,
    );

    const viewport = getScrollViewport(container);

    expect(viewport.className).toContain("min-h-0");
    expect(viewport.className).toContain("flex-1");
    expect(viewport.getAttribute("data-trace-card-scroll")).toBe("");
  });

  it("allows overflow content to scroll inside the viewport", () => {
    const { container } = render(
      <ScrollArea className="h-24 w-full">
        <div data-testid="scroll-content" style={{ height: "240px" }}>
          Tall content
        </div>
      </ScrollArea>,
    );

    const viewport = constrainScrollViewport(
      getScrollViewport(container),
      96,
      240,
    );

    expect(viewport.scrollHeight).toBeGreaterThan(viewport.clientHeight);

    viewport.scrollTop = 48;
    expect(viewport.scrollTop).toBe(48);
  });

  it("keeps keyboard focus and scrolling on nested interactive content", async () => {
    const user = userEvent.setup();

    const { container } = render(
      <ScrollArea className="h-24 w-full">
        <div style={{ height: "240px" }}>
          <input aria-label="Scrollable field" />
        </div>
      </ScrollArea>,
    );

    const viewport = constrainScrollViewport(
      getScrollViewport(container),
      96,
      240,
    );
    const field = screen.getByRole("textbox", { name: "Scrollable field" });

    await user.tab();
    expect(field).toHaveFocus();

    viewport.scrollTop = 0;
    field.focus();
    field.dispatchEvent(
      new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }),
    );

    expect(document.activeElement).toBe(field);
    expect(viewport.scrollTop).toBeGreaterThanOrEqual(0);
  });
});
