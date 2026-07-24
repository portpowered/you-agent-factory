// @vitest-environment happy-dom

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
} from "@you-agent-factory/components";
import {
  Collapsible as DeepCollapsible,
  Dialog as DeepDialog,
  Popover as DeepPopover,
  ScrollArea as DeepScrollArea,
} from "@you-agent-factory/components/overlays";
import { axe } from "jest-axe";
import { describe, expect, it } from "vitest";
import { renderPackageComponent, screen, userEvent } from "../testing/render";

describe("overlay primitives from @you-agent-factory/components", () => {
  it("renders Dialog through the root package import", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <main>
        <Dialog>
          <DialogTrigger>Open settings</DialogTrigger>
          <DialogContent
            aria-describedby={undefined}
            closeLabel="Dismiss dialog"
          >
            <DialogTitle>Settings</DialogTitle>
            <DialogDescription>
              Adjust package-owned dialog content.
            </DialogDescription>
          </DialogContent>
        </Dialog>
      </main>,
    );

    await user.click(screen.getByRole("button", { name: "Open settings" }));
    expect(
      screen.getByRole("dialog", { name: "Settings" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Dismiss dialog" }),
    ).toBeInTheDocument();
    expect(
      await axe(document.body, {
        rules: {
          "aria-hidden-focus": { enabled: false },
        },
      }),
    ).toHaveNoViolations();
  });

  it("renders Popover through the root package import", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <main>
        <Popover>
          <PopoverTrigger>Show hint</PopoverTrigger>
          <PopoverContent aria-label="Package hint">
            <p>Package popover content</p>
          </PopoverContent>
        </Popover>
      </main>,
    );

    await user.click(screen.getByRole("button", { name: "Show hint" }));
    expect(screen.getByText("Package popover content")).toBeInTheDocument();
    expect(
      await axe(document.body, {
        rules: {
          "aria-hidden-focus": { enabled: false },
        },
      }),
    ).toHaveNoViolations();
  });

  it("renders Collapsible through the root package import", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <main>
        <Collapsible>
          <CollapsibleTrigger>More details</CollapsibleTrigger>
          <CollapsibleContent>
            Expanded package collapsible content
          </CollapsibleContent>
        </Collapsible>
      </main>,
    );

    const trigger = screen.getByRole("button", { name: "More details" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByText("Expanded package collapsible content"),
    ).toBeInTheDocument();
    expect(await axe(document.body)).toHaveNoViolations();
  });

  it("renders ScrollArea through the root package import", () => {
    renderPackageComponent(
      <main>
        <ScrollArea className="h-24 w-48" data-testid="package-scroll-area">
          <div style={{ height: "240px" }}>Overflowing scroll content</div>
        </ScrollArea>
      </main>,
    );

    const root = screen.getByTestId("package-scroll-area");
    const viewport = root.querySelector("[data-radix-scroll-area-viewport]");

    expect(root.className).toContain("overflow-hidden");
    expect(viewport).toBeTruthy();
    expect(screen.getByText("Overflowing scroll content")).toBeInTheDocument();
  });
});

describe("overlay primitives from @you-agent-factory/components/overlays", () => {
  it("exposes the same primitives through the documented deep import", () => {
    expect(DeepDialog).toBe(Dialog);
    expect(DeepPopover).toBe(Popover);
    expect(DeepCollapsible).toBe(Collapsible);
    expect(DeepScrollArea).toBe(ScrollArea);
  });
});
