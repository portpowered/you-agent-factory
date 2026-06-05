import { fireEvent, render, screen } from "@testing-library/react";

import {
  FactoryGraphEditorSelectField,
  FactoryGraphEditorTextField,
} from "./factory-graph-editor-add-dialog-fields";

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
    expect(input.className).toContain("bg-surface");
    expect(screen.getByText("Names become factory node identifiers.")).toBeTruthy();
    expect(screen.getByText("Use a unique name.").className).toContain(
      "text-on-error-container",
    );

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
