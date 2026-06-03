import { fireEvent, render, screen, waitFor } from "@testing-library/react";

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

  it("does not surface prompt-style validation markers for guard selector text", async () => {
    const onChange = vi.fn();

    render(
      <MonacoGuardSelectorEditor
        ariaLabel="Field selector"
        loadingMessage="Starting selector editor."
        modelPath="inmemory://model/test/workstation-guard-selector/row-0"
        onChange={onChange}
        startupErrorMessage="Selector editor failed."
        value=""
      />,
    );

    const selectorEditor = screen.getByLabelText("Field selector");
    const wrapper = selectorEditor.parentElement;

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-marker-count")).toBe("0");
    });

    fireEvent.change(selectorEditor, {
      target: { value: '.Custom["not-in-suggestions"]' },
    });

    expect(onChange).toHaveBeenCalledWith('.Custom["not-in-suggestions"]');
    expect(wrapper?.getAttribute("data-monaco-marker-count")).toBe("0");
    expect(wrapper?.getAttribute("data-monaco-marker-messages")).toBe("[]");
  });

  it("routes mount-handler content changes and applies error styling", async () => {
    const onChange = vi.fn();

    render(
      <MonacoGuardSelectorEditor
        ariaLabel="Field selector"
        hasError
        loadingMessage="Starting selector editor."
        modelPath="inmemory://model/test/workstation-guard-selector/row-1"
        onChange={onChange}
        startupErrorMessage="Selector editor failed."
        value=""
      />,
    );

    const selectorEditor = screen.getByLabelText("Field selector");
    const wrapper = selectorEditor.parentElement;

    fireEvent.change(selectorEditor, { target: { value: "." } });
    fireEvent.change(selectorEditor, { target: { value: ".Na" } });

    await waitFor(() => {
      expect(onChange).toHaveBeenCalledWith(".");
      expect(onChange).toHaveBeenCalledWith(".Na");
    });
    expect(wrapper?.className).toContain("border-af-danger-border");
  });
});
