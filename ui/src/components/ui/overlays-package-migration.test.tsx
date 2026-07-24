import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
  Collapsible as DashboardCollapsible,
  CollapsibleContent as DashboardCollapsibleContent,
  CollapsibleTrigger as DashboardCollapsibleTrigger,
} from "./collapsible";
import {
  Dialog as DashboardDialog,
  DialogContent as DashboardDialogContent,
  DialogTitle as DashboardDialogTitle,
} from "./dialog";
import {
  Popover as DashboardPopover,
  PopoverContent as DashboardPopoverContent,
  PopoverTrigger as DashboardPopoverTrigger,
} from "./popover";
import { ScrollArea as DashboardScrollArea } from "./scroll-area";

describe("dashboard overlay package migration", () => {
  it("re-exports overlay primitives from @you-agent-factory/components", () => {
    expect(DashboardDialog).toBe(Dialog);
    expect(DashboardDialogContent).toBe(DialogContent);
    expect(DashboardDialogTitle).toBe(DialogTitle);
    expect(DashboardPopover).toBe(Popover);
    expect(DashboardPopoverContent).toBe(PopoverContent);
    expect(DashboardPopoverTrigger).toBe(PopoverTrigger);
    expect(DashboardCollapsible).toBe(Collapsible);
    expect(DashboardCollapsibleContent).toBe(CollapsibleContent);
    expect(DashboardCollapsibleTrigger).toBe(CollapsibleTrigger);
    expect(DashboardScrollArea).toBe(ScrollArea);
  });

  it("renders representative migrated Dialog, Popover, Collapsible, and ScrollArea consumers", async () => {
    const user = userEvent.setup();

    const { rerender } = render(
      <main>
        <Dialog>
          <DialogTrigger>Open export</DialogTrigger>
          <DialogContent aria-describedby={undefined} closeLabel="Close dialog">
            <DialogTitle>Export factory</DialogTitle>
            <DialogDescription>Confirm export details.</DialogDescription>
          </DialogContent>
        </Dialog>
      </main>,
    );

    await user.click(screen.getByRole("button", { name: "Open export" }));
    expect(
      screen.getByRole("dialog", { name: "Export factory" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Close dialog" }),
    ).toBeInTheDocument();

    rerender(
      <main>
        <Popover>
          <PopoverTrigger>Work type help</PopoverTrigger>
          <PopoverContent aria-label="Work type help">
            <p>Choose a work type for the request.</p>
          </PopoverContent>
        </Popover>
      </main>,
    );

    await user.click(screen.getByRole("button", { name: "Work type help" }));
    expect(
      screen.getByText("Choose a work type for the request."),
    ).toBeInTheDocument();

    rerender(
      <main>
        <Collapsible defaultOpen>
          <CollapsibleTrigger>Prompt details</CollapsibleTrigger>
          <CollapsibleContent>
            <p>Expanded prompt configuration</p>
          </CollapsibleContent>
        </Collapsible>

        <ScrollArea aria-label="Agent bento body" className="h-24">
          <div style={{ height: "160px" }}>Scrollable bento content</div>
        </ScrollArea>
      </main>,
    );

    expect(screen.getByText("Expanded prompt configuration")).toBeVisible();
    expect(screen.getByLabelText("Agent bento body")).toBeInTheDocument();
    expect(screen.getByText("Scrollable bento content")).toBeInTheDocument();
  });
});
