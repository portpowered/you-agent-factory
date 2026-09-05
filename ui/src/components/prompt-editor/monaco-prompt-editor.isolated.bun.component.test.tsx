import { beforeEach, describe, expect, it, mock } from "bun:test";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { useState } from "react";

import * as monacoEditorAll from "../../testing/mocks/monaco-editor-all";
import * as monacoEditorApi from "../../testing/mocks/monaco-editor-api";
import * as monacoReact from "../../testing/mocks/monaco-react";
import { AppColorPaletteProvider, useAppColorPalette } from "../../theme";

mock.module("@monaco-editor/react", () => monacoReact);
mock.module("monaco-editor/esm/vs/editor/editor.all.js", () => monacoEditorAll);
mock.module("monaco-editor/esm/vs/editor/editor.api.js", () => monacoEditorApi);

const { MonacoPromptEditor } = await import("./monaco-prompt-editor");
const { WORKSTATION_PROMPT_THEME_ID } = await import("./monaco-prompt-setup");
const vi = { fn: mock };

let setPaletteForTest:
  | ((palette: string | null | undefined) => void)
  | undefined;

function PaletteDriver() {
  setPaletteForTest = useAppColorPalette().setPalette;
  return null;
}

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
  setPaletteForTest = undefined;
  window.sessionStorage.clear();
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
      <AppColorPaletteProvider initialPalette="factory-dark">
        <PaletteDriver />
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
        />
      </AppColorPaletteProvider>,
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

  it("reapplies the selected palette without remounting or losing editor state", async () => {
    function PaletteBoundPromptEditor() {
      const [value, setValue] = useState("{{ .WorkID }}");

      return (
        <MonacoPromptEditor
          ariaLabel="Prompt"
          autocompleteState={readyAutocompleteState}
          loadingMessage="Loading prompt editor."
          modelPath="inmemory://model/test/workstation-prompt/palette-refresh"
          onChange={setValue}
          startupErrorMessage="Prompt editor failed."
          value={value}
        />
      );
    }

    render(
      <AppColorPaletteProvider initialPalette="factory-dark">
        <PaletteDriver />
        <PaletteBoundPromptEditor />
      </AppColorPaletteProvider>,
    );

    const promptEditor = screen.getByLabelText("Prompt");
    const wrapper = promptEditor.parentElement;

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-theme-application-count")).toBe(
        "1",
      );
    });

    fireEvent.change(promptEditor, { target: { value: "Updated prompt" } });
    promptEditor.focus();

    for (const [index, palette] of [
      "factory-light",
      "slate",
      "olive",
    ].entries()) {
      await act(async () => {
        setPaletteForTest?.(palette);
      });

      await waitFor(() => {
        expect(
          wrapper?.getAttribute("data-monaco-theme-application-count"),
        ).toBe(String(index + 2));
      });
    }

    expect(wrapper?.getAttribute("data-monaco-theme-bases")).toBe(
      JSON.stringify(["vs-dark", "vs", "vs-dark", "vs-dark"]),
    );
    expect(screen.getByLabelText("Prompt")).toBe(promptEditor);
    expect((promptEditor as HTMLTextAreaElement).value).toBe("Updated prompt");
    expect(document.activeElement).toBe(promptEditor);
  });
});
