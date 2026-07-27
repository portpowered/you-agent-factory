// @component-test-runner vitest: Bun 1.3.12 on Linux does not reliably flush Monaco palette MutationObservers.
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ComponentProps } from "react";
import { beforeEach } from "vitest";

import { MonacoGuardSelectorEditor } from "./monaco-guard-selector-editor";
import { WORKSTATION_GUARD_SELECTOR_THEME_ID } from "./monaco-guard-selector-setup";

const guardSelectorPaletteSequence = [
  { ink: "#F7F2E8", palette: "factory-dark", surface: "#181f2b" },
  { ink: "#091117", palette: "factory-light", surface: "#F7F2E8" },
  { ink: "#E6E0E9", palette: "material-baseline", surface: "#1D1B20" },
  { ink: "#E6EDF3", palette: "slate", surface: "#161B22" },
  { ink: "#EEF2E4", palette: "olive", surface: "#1A1D15" },
] as const;

function resetGuardSelectorPalette() {
  document.documentElement.removeAttribute("data-color-palette");
  document.documentElement.style.removeProperty("--color-surface");
  document.documentElement.style.removeProperty("--color-on-surface");
}

function applyGuardSelectorPalette({
  ink,
  palette,
  surface,
}: {
  ink: string;
  palette: string;
  surface: string;
}) {
  document.documentElement.dataset.colorPalette = palette;
  document.documentElement.style.setProperty("--color-surface", surface);
  document.documentElement.style.setProperty("--color-on-surface", ink);
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
    <MonacoGuardSelectorEditor
      ariaLabel="Field selector"
      loadingMessage="Starting selector editor."
      modelPath="inmemory://model/test/workstation-guard-selector/default"
      onChange={() => {}}
      startupErrorMessage="Selector editor failed."
      value=".Name"
      {...overrides}
    />,
  );
}

async function expectPaletteRefresh(
  wrapper: Element | null,
  editor: HTMLElement,
) {
  await waitFor(() => {
    expectSingleThemeApplication(wrapper);
  });

  for (const [index, palette] of guardSelectorPaletteSequence
    .slice(1)
    .entries()) {
    applyGuardSelectorPalette(palette);

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
}

describe("MonacoGuardSelectorEditor", () => {
  beforeEach(resetGuardSelectorPalette);

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

  it("redefines and reapplies the guard-selector theme when the dashboard palette changes", async () => {
    applyGuardSelectorPalette(guardSelectorPaletteSequence[0]);
    renderGuardSelectorEditor({
      modelPath:
        "inmemory://model/test/workstation-guard-selector/palette-refresh",
    });

    const selectorEditor = screen.getByLabelText("Field selector");
    const wrapper = selectorEditor.parentElement;

    await expectPaletteRefresh(wrapper, selectorEditor);
  });
});
