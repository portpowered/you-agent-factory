// @vitest-environment happy-dom

import { describe, expect, it, vi } from "vitest";

import { renderPackageComponent, screen, userEvent } from "../testing/render";
import {
  WidgetFrameDisclosure,
  WidgetFrameDisclosurePanel,
  WidgetFrameDisclosureTrigger,
} from "./widget-frame-disclosure";

describe("WidgetFrameDisclosure", () => {
  it("exposes disclosure semantics and icon rotation when collapsed", () => {
    renderPackageComponent(
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          controlsID="details-panel"
          expanded={false}
        >
          Show details
        </WidgetFrameDisclosureTrigger>
        <WidgetFrameDisclosurePanel expanded={false} id="details-panel">
          <p>Hidden details</p>
        </WidgetFrameDisclosurePanel>
      </WidgetFrameDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: "Show details" });

    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(trigger.getAttribute("aria-controls")).toBe("details-panel");
    expect(trigger.className).toContain("focus-visible:outline-af-accent");
    expect(trigger.querySelector("svg")?.className).toContain("rotate-0");
    expect(screen.queryByText("Hidden details", { hidden: true })).toBeTruthy();
    expect(
      document.getElementById("details-panel")?.hasAttribute("hidden"),
    ).toBe(true);
  });

  it("exposes disclosure semantics and rotated icon when expanded", () => {
    renderPackageComponent(
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          aria-label="Collapse details"
          controlsID="details-panel"
          expanded
        />
        <WidgetFrameDisclosurePanel expanded id="details-panel">
          <p>Visible details</p>
        </WidgetFrameDisclosurePanel>
      </WidgetFrameDisclosure>,
    );

    const trigger = screen.getByRole("button", { name: "Collapse details" });

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(trigger.querySelector("svg")?.className).toContain("rotate-180");
    expect(screen.getByText("Visible details")).toBeVisible();
  });

  it("invokes onExpandedChange with the next expanded state", async () => {
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();

    renderPackageComponent(
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          controlsID="details-panel"
          expanded={false}
          onExpandedChange={onExpandedChange}
        >
          Show details
        </WidgetFrameDisclosureTrigger>
      </WidgetFrameDisclosure>,
    );

    await user.click(screen.getByRole("button", { name: "Show details" }));

    expect(onExpandedChange).toHaveBeenCalledWith(true);
  });

  it("does not toggle when click default is prevented", async () => {
    const user = userEvent.setup();
    const onExpandedChange = vi.fn();

    renderPackageComponent(
      <WidgetFrameDisclosure>
        <WidgetFrameDisclosureTrigger
          controlsID="details-panel"
          expanded={false}
          onClick={(event) => event.preventDefault()}
          onExpandedChange={onExpandedChange}
        >
          Show details
        </WidgetFrameDisclosureTrigger>
      </WidgetFrameDisclosure>,
    );

    await user.click(screen.getByRole("button", { name: "Show details" }));

    expect(onExpandedChange).not.toHaveBeenCalled();
  });
});
