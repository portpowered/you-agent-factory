// @vitest-environment happy-dom

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Dialog,
  DialogContent,
  DialogTitle,
  DialogTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
  ScrollArea,
} from "@you-agent-factory/components";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import {
  renderPackageComponent,
  screen,
  userEvent,
  waitFor,
} from "../testing/render";

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

function ControlledDialogExample() {
  const [open, setOpen] = useState(false);

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger>Open controlled dialog</DialogTrigger>
      <DialogContent aria-describedby={undefined} closeLabel="Close dialog">
        <DialogTitle>Controlled dialog</DialogTitle>
        <button type="button">Dialog action</button>
      </DialogContent>
    </Dialog>
  );
}

function ControlledPopoverExample() {
  const [open, setOpen] = useState(false);

  return (
    <main>
      <button type="button">Outside surface</button>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger>Show controlled popover</PopoverTrigger>
        <PopoverContent aria-label="Controlled popover">
          <p>Controlled popover content</p>
        </PopoverContent>
      </Popover>
    </main>
  );
}

function ControlledCollapsibleExample() {
  const [open, setOpen] = useState(false);

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger>Controlled details</CollapsibleTrigger>
      <CollapsibleContent>Controlled collapsible content</CollapsibleContent>
    </Collapsible>
  );
}

function isFocusWithinDialog(dialog: HTMLElement): boolean {
  const activeElement = document.activeElement;
  return activeElement instanceof HTMLElement && dialog.contains(activeElement);
}

describe("Dialog focus and keyboard behavior", () => {
  it("moves focus into the dialog, traps focus, closes on Escape, and returns focus to the trigger", async () => {
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
            <button type="button">First action</button>
            <button type="button">Second action</button>
          </DialogContent>
        </Dialog>
      </main>,
    );

    const trigger = screen.getByRole("button", { name: "Open settings" });
    trigger.focus();
    await user.click(trigger);

    const dialog = screen.getByRole("dialog", { name: "Settings" });
    expect(dialog).toBeInTheDocument();
    expect(isFocusWithinDialog(dialog)).toBe(true);

    const firstAction = screen.getByRole("button", { name: "First action" });
    const secondAction = screen.getByRole("button", { name: "Second action" });
    const closeButton = screen.getByRole("button", { name: "Dismiss dialog" });

    await user.tab();
    expect(
      [firstAction, secondAction, closeButton, trigger].includes(
        document.activeElement as HTMLElement,
      ),
    ).toBe(true);
    expect(isFocusWithinDialog(dialog)).toBe(true);

    await user.tab();
    expect(isFocusWithinDialog(dialog)).toBe(true);

    await user.keyboard("{Escape}");

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Settings" })).toBeNull();
    });
    expect(document.activeElement).toBe(trigger);
  });

  it("supports controlled open state from the host", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledDialogExample />);

    const trigger = screen.getByRole("button", {
      name: "Open controlled dialog",
    });
    expect(
      screen.queryByRole("dialog", { name: "Controlled dialog" }),
    ).toBeNull();

    await user.click(trigger);
    expect(
      screen.getByRole("dialog", { name: "Controlled dialog" }),
    ).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(
        screen.queryByRole("dialog", { name: "Controlled dialog" }),
      ).toBeNull();
    });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });
});

describe("Popover focus and keyboard behavior", () => {
  it("closes on Escape, outside interaction, and returns focus to the trigger", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <main>
        <button type="button">Outside surface</button>
        <Popover>
          <PopoverTrigger>Show hint</PopoverTrigger>
          <PopoverContent aria-label="Package hint">
            <p>Package popover content</p>
          </PopoverContent>
        </Popover>
      </main>,
    );

    const trigger = screen.getByRole("button", { name: "Show hint" });
    trigger.focus();
    await user.click(trigger);

    expect(screen.getByText("Package popover content")).toBeInTheDocument();
    expect(trigger).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByText("Package popover content")).toBeNull();
    });
    expect(document.activeElement).toBe(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);
    expect(screen.getByText("Package popover content")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Outside surface" }));
    await waitFor(() => {
      expect(screen.queryByText("Package popover content")).toBeNull();
    });
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Outside surface" }),
    );
  });

  it("supports controlled open state from the host", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledPopoverExample />);

    const trigger = screen.getByRole("button", {
      name: "Show controlled popover",
    });
    expect(screen.queryByText("Controlled popover content")).toBeNull();

    await user.click(trigger);
    expect(screen.getByText("Controlled popover content")).toBeInTheDocument();
    expect(trigger).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByText("Controlled popover content")).toBeNull();
    });
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });
});

describe("Collapsible keyboard and disclosure behavior", () => {
  it("toggles with keyboard input and exposes expanded state to assistive technology", async () => {
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
    trigger.focus();
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.keyboard("{Enter}");
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByText("Expanded package collapsible content"),
    ).toBeInTheDocument();

    await user.keyboard(" ");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(
      screen.queryByText("Expanded package collapsible content"),
    ).toBeNull();
  });

  it("supports controlled open state from the host", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledCollapsibleExample />);

    const trigger = screen.getByRole("button", { name: "Controlled details" });
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(
      screen.getByText("Controlled collapsible content"),
    ).toBeInTheDocument();

    await user.click(trigger);
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(screen.queryByText("Controlled collapsible content")).toBeNull();
  });
});

describe("ScrollArea keyboard reachability", () => {
  it("keeps nested focusable content reachable without hiding focus inside the viewport", async () => {
    const user = userEvent.setup();

    const { container } = renderPackageComponent(
      <main>
        <ScrollArea className="h-24 w-full" data-testid="package-scroll-area">
          <div style={{ height: "240px" }}>
            <input aria-label="Scrollable field" />
          </div>
        </ScrollArea>
      </main>,
    );

    const viewport = constrainScrollViewport(
      getScrollViewport(container),
      96,
      240,
    );
    const field = screen.getByRole("textbox", { name: "Scrollable field" });

    expect(viewport.className).toContain("outline-none");

    await user.tab();
    expect(field).toHaveFocus();
    expect(document.activeElement).toBe(field);
  });
});
