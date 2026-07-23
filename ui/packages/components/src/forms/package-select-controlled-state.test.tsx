// @vitest-environment happy-dom

import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installPackageBrowserTestShims } from "../testing/package-browser-shims";
import {
  renderPackageComponent,
  screen,
  userEvent,
  within,
} from "../testing/render";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";

function WorkTypeSelect({
  disabled = false,
  onOpenChange,
  onValueChange,
  open,
  value,
}: {
  disabled?: boolean;
  onOpenChange?: (open: boolean) => void;
  onValueChange?: (value: string) => void;
  open?: boolean;
  value?: string;
}) {
  return (
    <Select
      disabled={disabled}
      onOpenChange={onOpenChange}
      onValueChange={onValueChange}
      open={open}
      value={value}
    >
      <SelectTrigger aria-label="Work type" id="work-type-select">
        <SelectValue placeholder="Select a work type" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="story">Story</SelectItem>
        <SelectItem disabled value="task">
          Task
        </SelectItem>
        <SelectItem value="bug">Bug</SelectItem>
      </SelectContent>
    </Select>
  );
}

function StatefulWorkTypeSelect({
  defaultValue,
  onValueChange,
}: {
  defaultValue?: string;
  onValueChange?: (value: string) => void;
}) {
  const [value, setValue] = useState(defaultValue);

  return (
    <WorkTypeSelect
      onValueChange={(nextValue) => {
        setValue(nextValue);
        onValueChange?.(nextValue);
      }}
      value={value}
    />
  );
}

function ControlledOpenHarness({
  onOpenChange,
}: {
  onOpenChange?: (open: boolean) => void;
}) {
  const [open, setOpen] = useState(false);

  return (
    <WorkTypeSelect
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        onOpenChange?.(nextOpen);
      }}
      open={open}
      value="story"
    />
  );
}

function HostControlledWorkTypeSelect({
  value,
}: {
  value: string | undefined;
}) {
  const [currentValue, setCurrentValue] = useState(value);

  return (
    <>
      <button onClick={() => setCurrentValue("bug")} type="button">
        Set bug
      </button>
      <WorkTypeSelect onValueChange={setCurrentValue} value={currentValue} />
    </>
  );
}

function useSelectControlledStateSuite() {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installPackageBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });
}

describe("Select controlled value", () => {
  useSelectControlledStateSuite();

  it("reflects host-provided value changes", () => {
    const onValueChange = vi.fn();

    const { rerender } = renderPackageComponent(
      <WorkTypeSelect onValueChange={onValueChange} value="story" />,
    );

    expect(
      screen.getByRole("combobox", { name: "Work type" }),
    ).toHaveTextContent("Story");

    rerender(<WorkTypeSelect onValueChange={onValueChange} value="bug" />);

    expect(
      screen.getByRole("combobox", { name: "Work type" }),
    ).toHaveTextContent("Bug");
  });

  it("calls onValueChange only when selecting a different enabled option", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <WorkTypeSelect onValueChange={onValueChange} value="story" />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    await user.click(trigger);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Story",
      }),
    );

    expect(onValueChange).not.toHaveBeenCalled();

    await user.click(trigger);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Bug",
      }),
    );

    expect(onValueChange).toHaveBeenCalledTimes(1);
    expect(onValueChange).toHaveBeenCalledWith("bug");
  });

  it("does not select disabled options through pointer interaction", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <WorkTypeSelect onValueChange={onValueChange} value="story" />,
    );

    await user.click(screen.getByRole("combobox", { name: "Work type" }));
    const listbox = await screen.findByRole("listbox");
    const disabledOption = within(listbox).getByRole("option", {
      name: "Task",
    });

    expect(disabledOption).toHaveAttribute("aria-disabled", "true");
    await user.click(disabledOption);

    expect(onValueChange).not.toHaveBeenCalled();
    await user.keyboard("{Escape}");
    expect(
      screen.getByRole("combobox", { name: "Work type" }),
    ).toHaveTextContent("Story");
  });

  it("skips disabled options during keyboard navigation", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <StatefulWorkTypeSelect
        defaultValue="story"
        onValueChange={onValueChange}
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    trigger.focus();
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(onValueChange).toHaveBeenCalledWith("bug");
    expect(trigger).toHaveTextContent("Bug");
    expect(trigger).toHaveFocus();
  });
});

describe("Select controlled open and keyboard focus", () => {
  useSelectControlledStateSuite();

  it("reflects host-provided open state", () => {
    const onOpenChange = vi.fn();

    const { rerender } = renderPackageComponent(
      <WorkTypeSelect onOpenChange={onOpenChange} open={false} value="story" />,
    );

    expect(screen.queryByRole("listbox")).toBeNull();

    rerender(<WorkTypeSelect onOpenChange={onOpenChange} open value="story" />);

    expect(screen.getByRole("listbox")).toBeTruthy();
  });

  it("calls onOpenChange when the user opens and closes the menu", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();

    renderPackageComponent(
      <ControlledOpenHarness onOpenChange={onOpenChange} />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");

    expect(onOpenChange).toHaveBeenCalledWith(true);
    expect(screen.getByRole("listbox")).toBeTruthy();

    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("opens with Space on a focused trigger and returns focus after selection", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<StatefulWorkTypeSelect defaultValue="story" />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    trigger.focus();
    await user.keyboard(" ");

    const listbox = await screen.findByRole("listbox");
    await user.click(within(listbox).getByRole("option", { name: "Bug" }));

    expect(trigger).toHaveTextContent("Bug");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it("exposes focus-visible ring treatment on the trigger", () => {
    renderPackageComponent(<WorkTypeSelect value="story" />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger.className).toContain("focus-visible:ring-2");
    expect(trigger.className).toContain("focus-visible:ring-af-focus-ring");
  });

  it("updates when the host changes the controlled value through surrounding state", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<HostControlledWorkTypeSelect value="story" />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    expect(trigger).toHaveTextContent("Story");

    await user.click(screen.getByRole("button", { name: "Set bug" }));

    expect(trigger).toHaveTextContent("Bug");
  });
});
