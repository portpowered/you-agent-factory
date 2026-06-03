import { fireEvent, render, screen } from "@testing-library/react";

import { MonacoGuardSelectorEditor } from "./monaco-guard-selector-editor";

describe("MonacoGuardSelectorEditor", () => {
  it("wires accessibility props, editing, and guard-selector surface marker", () => {
    const onChange = vi.fn();

    render(
      <MonacoGuardSelectorEditor
        ariaDescribedBy="guard-selector-error"
        ariaInvalid
        ariaLabel="Field selector"
        loadingMessage="Starting selector editor."
        modelPath="inmemory://model/test/workstation-guard-selector/row-0"
        onChange={onChange}
        startupErrorMessage="Selector editor failed."
        value=".Name"
      />,
    );

    const selectorEditor = screen.getByLabelText("Field selector");
    const wrapper = selectorEditor.parentElement;

    expect((selectorEditor as HTMLTextAreaElement).value).toBe(".Name");
    expect(wrapper?.getAttribute("data-monaco-editor")).toBe(
      "workstation-guard-selector",
    );
    expect(wrapper?.getAttribute("aria-describedby")).toBe(
      "guard-selector-error",
    );
    expect(wrapper?.getAttribute("aria-invalid")).toBe("true");

    fireEvent.change(selectorEditor, { target: { value: ".WorkID" } });

    expect(onChange).toHaveBeenCalledWith(".WorkID");
  });
});
