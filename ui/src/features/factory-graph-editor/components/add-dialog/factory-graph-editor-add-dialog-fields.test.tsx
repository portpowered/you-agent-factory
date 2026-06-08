import { fireEvent, render, screen } from "@testing-library/react";

import {
  FactoryGraphEditorSelectField,
  FactoryGraphEditorTextField,
} from "../add-dialog/factory-graph-editor-add-dialog-fields";

describe("FactoryGraphEditor add dialog fields", () => {
  it("renders text field label, help, error, and tokenized input surface", () => {
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

    fireEvent.change(input, { target: { value: "beta" } });
    expect(onChange).toHaveBeenCalledWith("beta");
  });

  it("renders select options and forwards value changes", () => {
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

    fireEvent.change(screen.getByLabelText("Node kind"), {
      target: { value: "worker" },
    });

    expect(onChange).toHaveBeenCalledWith("worker");
  });
});
