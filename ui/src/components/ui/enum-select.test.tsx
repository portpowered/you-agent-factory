import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../dashboard/test-browser-shims";
import { EnumSelect, OptionalEnumSelect, ResetEnumSelect } from "./enum-select";

const priorityOptions = [
  { label: "Low", value: "low" },
  { label: "High", value: "high" },
] as const;

describe("EnumSelect helpers", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    cleanup();
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("selects a required enum value through the combobox", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();

    render(
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

    render(
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

    render(
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
