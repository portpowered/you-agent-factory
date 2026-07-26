import { beforeEach, describe, expect, it, mock } from "bun:test";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import * as monacoEditorAll from "../../testing/mocks/monaco-editor-all";
import * as monacoEditorApi from "../../testing/mocks/monaco-editor-api";
import * as monacoReact from "../../testing/mocks/monaco-react";

mock.module("@monaco-editor/react", () => monacoReact);
mock.module("monaco-editor/esm/vs/editor/editor.all.js", () => monacoEditorAll);
mock.module("monaco-editor/esm/vs/editor/editor.api.js", () => monacoEditorApi);

const { MonacoPromptEditor } = await import("./monaco-prompt-editor");
const { WORKSTATION_PROMPT_THEME_ID } = await import("./monaco-prompt-setup");
const vi = { fn: mock };

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

function resetPromptEditorPalette() {
  document.documentElement.removeAttribute("data-color-palette");
  document.documentElement.style.removeProperty("--color-surface");
  document.documentElement.style.removeProperty("--color-on-surface");
}

function expectSingleThemeApplication(wrapper: Element | null) {
  expect(wrapper?.getAttribute("data-monaco-theme-application-count")).toBe(
    "1",
  );
  expect(wrapper?.getAttribute("data-monaco-theme-set-count")).toBe("1");
  expect(wrapper?.getAttribute("data-monaco-theme-bases")).toBe(
    JSON.stringify(["vs-dark"]),
  );
  expect(wrapper?.getAttribute("data-monaco-theme-set-names")).toBe(
    JSON.stringify([WORKSTATION_PROMPT_THEME_ID]),
  );
}

describe("MonacoPromptEditor", () => {
  beforeEach(resetPromptEditorPalette);

  it("wires Monaco markers, accessibility props, editing, scroll, and ready lifecycle", async () => {
    const onChange = vi.fn();
    const onMount = vi.fn();
    const onReadyChange = vi.fn();
    const onScrollChange = vi.fn();

    const { unmount } = render(
      <MonacoPromptEditor
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
        modelPath="inmemory://model/test/workstation-prompt"
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
    expectSingleThemeApplication(wrapper);
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
