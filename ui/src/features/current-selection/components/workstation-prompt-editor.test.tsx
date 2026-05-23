import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import { WorkstationPromptEditor } from "./workstation-prompt-editor";

const readyAutocompleteState = {
  contract: {
    availableVariables: [
      {
        category: "ROOT",
        description: "The current work item identifier.",
        example: "{{ .WorkID }}",
        path: ".WorkID",
      },
    ],
    inputCount: 1,
    unavailableAccessPatterns: [],
  },
  status: "ready" as const,
};

describe("WorkstationPromptEditor", () => {
  it("wires Monaco markers, accessibility props, editing, scroll, and ready lifecycle", async () => {
    const onChange = vi.fn();
    const onMount = vi.fn();
    const onReadyChange = vi.fn();
    const onScrollChange = vi.fn();

    const { unmount } = render(
      <WorkstationPromptEditor
        ariaDescribedBy="prompt-help prompt-error"
        ariaInvalid
        ariaLabel="Prompt"
        autocompleteState={readyAutocompleteState}
        diagnostics={[
          {
            endOffset: 13,
            kind: "INVALID_VARIABLE",
            message: "Work ID is invalid.",
            sourceText: "{{ .WorkID }}",
            startOffset: 1,
          },
        ]}
        hasDiagnostics
        loadingMessage="Loading prompt editor."
        onChange={onChange}
        onMount={onMount}
        onReadyChange={onReadyChange}
        onScrollChange={onScrollChange}
        startupErrorMessage="Prompt editor failed."
        value="{{ .WorkID }}"
      />,
    );

    const promptEditor = screen.getByLabelText("Prompt");
    const wrapper = promptEditor.parentElement;

    expect((promptEditor as HTMLTextAreaElement).value).toBe("{{ .WorkID }}");
    expect(wrapper?.getAttribute("aria-describedby")).toBe(
      "prompt-help prompt-error",
    );
    expect(wrapper?.getAttribute("aria-invalid")).toBe("true");

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-marker-count")).toBe("1");
    });
    expect(wrapper?.getAttribute("data-monaco-marker-messages")).toContain(
      "Work ID is invalid.",
    );
    expect(wrapper?.className).toContain("border-af-danger-border");
    expect(onMount).toHaveBeenCalledTimes(1);
    expect(onReadyChange).toHaveBeenCalledWith(true);
    expect(onScrollChange).toHaveBeenCalledWith({
      scrollLeft: 0,
      scrollTop: 0,
    });
    expect(onScrollChange).toHaveBeenCalledWith({
      scrollLeft: 3,
      scrollTop: 4,
    });

    fireEvent.change(promptEditor, { target: { value: "Updated prompt" } });

    expect(onChange).toHaveBeenCalledWith("Updated prompt");

    unmount();

    expect(onReadyChange).toHaveBeenCalledWith(false);
  });
});
