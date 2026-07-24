// @vitest-environment happy-dom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installPackageBrowserTestShims } from "../testing/package-browser-shims";
import {
  renderPackageComponent,
  screen,
  userEvent,
  within,
} from "../testing/render";
import {
  EnumSelect,
  OptionalEnumSelect,
  ResetEnumSelect,
} from "./package-enum-select";
import { NativeSelect } from "./package-native-select";

const priorityOptions = [
  { label: "Low", value: "low" },
  { label: "High", value: "high" },
] as const;

describe("EnumSelect helpers", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installPackageBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("selects a required enum value through the combobox", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <EnumSelect
        aria-label="Priority"
        id="priority"
        onValueChange={onValueChange}
        options={priorityOptions}
        value="low"
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Priority" }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "High",
      }),
    );

    expect(onValueChange).toHaveBeenCalledWith("high");
  });

  it("clears optional enum values through the empty option", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <OptionalEnumSelect
        aria-label="Priority"
        emptyOptionLabel="Not configured"
        id="priority"
        onValueChange={onValueChange}
        options={priorityOptions}
        value="low"
      />,
    );

    await user.click(screen.getByRole("combobox", { name: "Priority" }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Not configured",
      }),
    );

    expect(onValueChange).toHaveBeenCalledWith(null);
  });

  it("resets after selecting a value in reset mode", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    renderPackageComponent(
      <ResetEnumSelect
        aria-label="Add item"
        id="add-item"
        onValueChange={onValueChange}
        options={priorityOptions}
        placeholder="Choose priority"
      />,
    );

    const trigger = screen.getByRole("combobox", { name: "Add item" });
    expect(trigger).toHaveTextContent("Choose priority");

    await user.click(trigger);
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "High",
      }),
    );

    expect(onValueChange).toHaveBeenCalledWith("high");
    expect(
      screen.getByRole("combobox", { name: "Add item" }),
    ).toHaveTextContent("Choose priority");
  });
});

describe("NativeSelect", () => {
  it("renders a native select with shared field styling", () => {
    renderPackageComponent(
      <NativeSelect aria-label="Status">
        <option value="open">Open</option>
        <option value="closed">Closed</option>
      </NativeSelect>,
    );

    const select = screen.getByLabelText("Status");
    expect(select.className).toContain("border-outline");
    expect(select.className).toContain("appearance-none");
    expect(select.className).toContain("pr-10");
  });
});
