import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installDashboardBrowserTestShims } from "../../../../components/dashboard/test-browser-shims";
import { selectLabeledComboboxOption } from "../../../../testing/select-test-helpers";

import {
  FactoryGraphEditorSelectField,
  FactoryGraphEditorTextField,
} from "./factory-graph-editor-add-dialog-fields";

let restoreBrowserShims: (() => void) | undefined;

beforeEach(() => {
  restoreBrowserShims = installDashboardBrowserTestShims();
});

afterEach(() => {
  cleanup();
  restoreBrowserShims?.();
  restoreBrowserShims = undefined;
});

describe("FactoryGraphEditor add dialog fields", () => {
  it("renders text field label, help, error, and tokenized input surface", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <FactoryGraphEditorTextField
        error="Use a unique name."
        helpText="Names become factory node identifiers."
        inputId="factory-graph-add-test-name"
        label="Node name"
        onChange={onChange}
        value="alpha"
      />,
    );

    const input = screen.getByLabelText("Node name");
    const help = screen.getByText("Names become factory node identifiers.");
    const error = screen.getByRole("alert");

    expect(input.className).toContain("bg-surface");
    expect(screen.getByText("Node name").className).toContain("font-semibold");
    expect(screen.getByText("Node name").className).toContain(
      "text-on-surface",
    );
    expect(help.className).toContain("af-dashboard-supporting-text");
    expect(error.textContent).toBe("Use a unique name.");
    expect(error.className).toContain("af-dashboard-supporting-text");
    expect(error.className).toContain("font-medium");
    expect(error.className).toContain("text-on-error-container");

    await user.clear(input);
    await user.type(input, "beta");
    expect(onChange).toHaveBeenCalled();
  });

  it("renders select options and forwards value changes", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <FactoryGraphEditorSelectField
        inputId="factory-graph-add-test-kind"
        label="Node kind"
        onChange={onChange}
        options={[
          { label: "Workstation", value: "workstation" },
          { label: "Worker", value: "worker" },
        ]}
        value="workstation"
      />,
    );

    await selectLabeledComboboxOption(user, "Node kind", "Worker");

    expect(onChange).toHaveBeenCalledWith("worker");
  });
});
