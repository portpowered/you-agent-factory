// @vitest-environment happy-dom

import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

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
  SelectField,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./package-select";

function ControlledWorkTypeSelect({
  defaultValue,
  disabled = false,
  error,
  triggerId = "work-type-select",
}: {
  defaultValue?: string;
  disabled?: boolean;
  error?: string;
  triggerId?: string;
}) {
  const [value, setValue] = useState(defaultValue);

  return (
    <SelectField
      description="Choose the work item category."
      descriptionId="work-type-description"
      error={error}
      errorId="work-type-error"
      inputId={triggerId}
      label="Work type"
    >
      <Select disabled={disabled} onValueChange={setValue} value={value}>
        <SelectTrigger
          aria-describedby={
            error
              ? "work-type-description work-type-error"
              : "work-type-description"
          }
          aria-invalid={error ? "true" : undefined}
          id={triggerId}
        >
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
    </SelectField>
  );
}

describe("Select primitives", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installPackageBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders placeholder text and associates the visible label", () => {
    renderPackageComponent(<ControlledWorkTypeSelect />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger).toHaveAttribute("id", "work-type-select");
    expect(screen.getByText("Select a work type")).toBeTruthy();
    expect(screen.getByText("Choose the work item category.")).toBeTruthy();
  });

  it("opens on trigger click and selects an option with the keyboard", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledWorkTypeSelect />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    await user.click(trigger);

    const listbox = await screen.findByRole("listbox");
    const storyOption = within(listbox).getByRole("option", { name: "Story" });

    storyOption.focus();
    await user.keyboard("{Enter}");

    expect(trigger).toHaveTextContent("Story");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("moves through options with arrow keys and closes on Escape", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledWorkTypeSelect defaultValue="story" />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    trigger.focus();
    await user.keyboard("{ArrowDown}");

    const listbox = await screen.findByRole("listbox");
    expect(
      within(listbox).getByRole("option", { name: "Story" }),
    ).toHaveAttribute("data-state", "checked");

    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(trigger).toHaveTextContent("Bug");
    expect(screen.queryByRole("listbox")).toBeNull();

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("keeps disabled triggers and options non-interactive", async () => {
    const user = userEvent.setup();

    renderPackageComponent(<ControlledWorkTypeSelect disabled />);

    const trigger = screen.getByRole("combobox", { name: "Work type" });

    expect(trigger).toHaveAttribute("disabled");

    await user.click(trigger);
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("renders validation errors with alert semantics", () => {
    renderPackageComponent(
      <ControlledWorkTypeSelect error="Work type is required." />,
    );

    const trigger = screen.getByRole("combobox", { name: "Work type" });
    const error = screen.getByRole("alert");

    expect(trigger).toHaveAttribute("aria-invalid", "true");
    expect(error).toHaveTextContent("Work type is required.");
    expect(trigger).toHaveAttribute(
      "aria-describedby",
      "work-type-description work-type-error",
    );
  });
});
