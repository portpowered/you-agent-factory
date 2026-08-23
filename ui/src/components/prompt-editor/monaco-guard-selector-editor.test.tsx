import { beforeEach, describe, expect, it, mock } from "bun:test";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ComponentProps } from "react";

import { AppColorPaletteProvider, useAppColorPalette } from "../../theme";
import { MonacoGuardSelectorEditor } from "./monaco-guard-selector-editor";
import { WORKSTATION_GUARD_SELECTOR_THEME_ID } from "./monaco-guard-selector-setup";

const guardSelectorPaletteSequence = [
  "factory-dark",
  "factory-light",
  "material-baseline",
  "slate",
  "olive",
] as const;

let setPaletteForTest:
  | ((palette: string | null | undefined) => void)
  | undefined;

function PaletteDriver() {
  setPaletteForTest = useAppColorPalette().setPalette;
  return null;
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
    JSON.stringify([WORKSTATION_GUARD_SELECTOR_THEME_ID]),
  );
}

function expectGuardSelectorThemeBases(
  wrapper: Element | null,
  bases: string[],
) {
  expect(wrapper?.getAttribute("data-monaco-theme-bases")).toBe(
    JSON.stringify(bases),
  );
  expect(wrapper?.getAttribute("data-monaco-theme-set-count")).toBe(
    String(bases.length),
  );
  expect(wrapper?.getAttribute("data-monaco-theme-set-names")).toBe(
    JSON.stringify(
      new Array(bases.length).fill(WORKSTATION_GUARD_SELECTOR_THEME_ID),
    ),
  );
}

function renderGuardSelectorEditor(
  overrides?: Partial<ComponentProps<typeof MonacoGuardSelectorEditor>>,
) {
  render(
    <AppColorPaletteProvider initialPalette="factory-dark">
      <PaletteDriver />
      <MonacoGuardSelectorEditor
        ariaLabel="Field selector"
        loadingMessage="Starting selector editor."
        modelPath="inmemory://model/test/workstation-guard-selector/default"
        onChange={() => {}}
        startupErrorMessage="Selector editor failed."
        value=".Name"
        {...overrides}
      />
    </AppColorPaletteProvider>,
  );
}

async function expectPaletteRefresh(
  wrapper: Element | null,
  editor: HTMLElement,
) {
  await waitFor(() => {
    expectSingleThemeApplication(wrapper);
  });

  editor.focus();

  for (const [index, palette] of guardSelectorPaletteSequence
    .slice(1)
    .entries()) {
    await act(async () => {
      setPaletteForTest?.(palette);
    });

    await waitFor(() => {
      expect(wrapper?.getAttribute("data-monaco-theme-application-count")).toBe(
        String(index + 2),
      );
    });
  }

  expectGuardSelectorThemeBases(wrapper, [
    "vs-dark",
    "vs",
    "vs-dark",
    "vs-dark",
    "vs-dark",
  ]);
  expect(screen.getByLabelText("Field selector")).toBe(editor);
  expect(document.activeElement).toBe(editor);
}

describe("MonacoGuardSelectorEditor", () => {
  beforeEach(() => {
    setPaletteForTest = undefined;
    window.sessionStorage.clear();
  });

  it("wires accessibility props, editing, and guard-selector surface marker", () => {
    const onChange = mock(() => {});

    renderGuardSelectorEditor({
      ariaDescribedBy: "guard-selector-error",
      ariaInvalid: true,
      onChange,
      value: ".Name",
    });

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
    const onChange = mock(() => {});

    renderGuardSelectorEditor({ onChange, value: "" });

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
    const onChange = mock(() => {});

    renderGuardSelectorEditor({ hasError: true, onChange, value: "" });

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

  it("redefines and reapplies the guard-selector theme when the dashboard palette changes", async () => {
    renderGuardSelectorEditor({
      modelPath:
        "inmemory://model/test/workstation-guard-selector/palette-refresh",
    });

    const selectorEditor = screen.getByLabelText("Field selector");
    const wrapper = selectorEditor.parentElement;

    await expectPaletteRefresh(wrapper, selectorEditor);
  });
});
