// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installPackageBrowserTestShims } from "../testing/package-browser-shims";
import {
  renderPackageComponent,
  screen,
  userEvent,
  within,
} from "../testing/render";
import { EnumSelect } from "./package-enum-select";
import {
  Select,
  SelectContent,
  SelectEmpty,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";

const LONG_LABEL =
  "A very long workstation label that should stay readable inside narrow form layouts";

describe("Select empty and loading edge states", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installPackageBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders a non-selectable empty state instead of interactive options", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <Select>
        <SelectTrigger aria-label="Work type">
          <SelectValue placeholder="Select a work type" />
        </SelectTrigger>
        <SelectContent>
          <SelectEmpty>No work types available</SelectEmpty>
        </SelectContent>
      </Select>,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    await user.click(trigger);

    const listbox = await screen.findByRole("listbox");
    const emptyOption = within(listbox).getByRole("option", {
      name: "No work types available",
    });

    expect(emptyOption).toHaveAttribute("aria-disabled", "true");
    await user.click(emptyOption);
    expect(trigger).toHaveTextContent("Select a work type");
  });

  it("blocks interaction while enum options are loading", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <EnumSelect
        aria-label="Work type"
        id="work-type"
        loading
        loadingLabel="Loading work types..."
        onValueChange={onValueChange}
        options={[{ label: "Story", value: "story" }]}
        value="story"
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger).toHaveAttribute("disabled");
    expect(trigger).toHaveAttribute("aria-busy", "true");
    expect(trigger).toHaveTextContent("Loading work types...");

    await user.click(trigger);
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it("shows a disabled empty-options affordance for enum selects with no options", async () => {
    const user = userEvent.setup();

    renderPackageComponent(
      <EnumSelect
        aria-label="Work type"
        emptyOptionsLabel="No work types available"
        id="work-type"
        onValueChange={vi.fn()}
        options={[]}
        value="story"
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    await user.click(trigger);

    const emptyOption = within(await screen.findByRole("listbox")).getByRole(
      "option",
      { name: "No work types available" },
    );

    expect(emptyOption).toHaveAttribute("aria-disabled", "true");
  });
});

describe("Select error and long-label edge states", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installPackageBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("exposes invalid semantics and visible error text together", () => {
    renderPackageComponent(
      <div className="flex w-full max-w-md flex-col gap-1.5">
        <label htmlFor="work-type">Work type</label>
        <Select>
          <SelectTrigger
            aria-describedby="work-type-error"
            aria-invalid
            aria-label="Work type"
            id="work-type"
          >
            <SelectValue placeholder="Select a work type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="story">Story</SelectItem>
          </SelectContent>
        </Select>
        <p id="work-type-error" role="alert">
          Work type is required.
        </p>
      </div>,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Work type is required.",
    );
    expect(trigger).toHaveAttribute("aria-describedby", "work-type-error");
  });

  it("constrains long selected values and option labels within the control", () => {
    renderPackageComponent(
      <Select defaultValue="long-story">
        <SelectTrigger aria-label="Work type" className="max-w-xs">
          <SelectValue placeholder="Select a work type" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="long-story">{LONG_LABEL}</SelectItem>
          <SelectItem value="bug">Bug</SelectItem>
        </SelectContent>
      </Select>,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger.className).toContain("[&>span]:line-clamp-1");
    expect(trigger).toHaveTextContent(LONG_LABEL);
  });
});
