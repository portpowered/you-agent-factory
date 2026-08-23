import { beforeEach, describe, expect, it } from "bun:test";
import { act, render, screen, waitFor } from "@testing-library/react";

import { AppColorPaletteProvider, useAppColorPalette } from "../../theme";
import { WORKSTATION_PROMPT_THEME_ID } from "./monaco-prompt-setup";
import { MonacoTextEditor } from "./monaco-text-editor";

let setPaletteForTest:
  | ((palette: string | null | undefined) => void)
  | undefined;

function PaletteDriver() {
  setPaletteForTest = useAppColorPalette().setPalette;
  return null;
}

describe("MonacoTextEditor", () => {
  beforeEach(() => {
    setPaletteForTest = undefined;
    window.sessionStorage.clear();
  });

  it("refreshes its mounted theme from application palette state", async () => {
    render(
      <AppColorPaletteProvider initialPalette="factory-dark">
        <PaletteDriver />
        <MonacoTextEditor
          ariaLabel="Factory documentation"
          loadingMessage="Loading document editor."
          modelPath="inmemory://model/test/factory-doc"
          onChange={() => {}}
          startupErrorMessage="Document editor failed."
          value="Factory documentation"
        />
      </AppColorPaletteProvider>,
    );

    const editor = screen.getByLabelText("Factory documentation");
    const wrapper = editor.parentElement;

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-theme-application-count")).toBe(
        "1",
      );
    });

    editor.focus();
    await act(async () => {
      setPaletteForTest?.("factory-light");
    });

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-theme-application-count")).toBe(
        "2",
      );
    });

    expect(wrapper?.getAttribute("data-monaco-theme-bases")).toBe(
      JSON.stringify(["vs-dark", "vs"]),
    );
    expect(wrapper?.getAttribute("data-monaco-theme-set-names")).toBe(
      JSON.stringify([
        WORKSTATION_PROMPT_THEME_ID,
        WORKSTATION_PROMPT_THEME_ID,
      ]),
    );
    expect(screen.getByLabelText("Factory documentation")).toBe(editor);
    expect(document.activeElement).toBe(editor);
  });
});
