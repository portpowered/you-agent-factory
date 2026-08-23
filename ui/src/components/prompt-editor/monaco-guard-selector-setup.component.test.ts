import { describe, expect, it, vi } from "vitest";

import { getColorPaletteTokens } from "../../theme/color-palette-tokens";
import {
  applyWorkstationGuardSelectorTheme,
  buildWorkstationGuardSelectorCompletionItems,
  registerWorkstationGuardSelectorCompletionProvider,
  registerWorkstationGuardSelectorMonaco,
  resetWorkstationGuardSelectorMonacoRegistrationForTests,
  WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID,
  WORKSTATION_GUARD_SELECTOR_THEME_ID,
} from "./monaco-guard-selector-setup";

describe("registerWorkstationGuardSelectorMonaco", () => {
  it("registers the guard-selector language and theme once", () => {
    const register = vi.fn();
    const setMonarchTokensProvider = vi.fn();
    const defineTheme = vi.fn();
    const setTheme = vi.fn();

    resetWorkstationGuardSelectorMonacoRegistrationForTests();

    registerWorkstationGuardSelectorMonaco({
      editor: { defineTheme, setTheme },
      languages: {
        register,
        setMonarchTokensProvider,
      },
    } as unknown as typeof import("monaco-editor"));
    registerWorkstationGuardSelectorMonaco({
      editor: { defineTheme, setTheme },
      languages: {
        register,
        setMonarchTokensProvider,
      },
    } as unknown as typeof import("monaco-editor"));

    expect(register).toHaveBeenCalledTimes(1);
    expect(register).toHaveBeenCalledWith({
      id: WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID,
    });
    expect(setMonarchTokensProvider).toHaveBeenCalledTimes(1);
    expect(defineTheme).toHaveBeenCalledTimes(1);
    expect(defineTheme).toHaveBeenCalledWith(
      WORKSTATION_GUARD_SELECTOR_THEME_ID,
      expect.objectContaining({
        base: "vs-dark",
        inherit: true,
      }),
    );
    expect(setTheme).toHaveBeenCalledTimes(1);
    expect(setTheme).toHaveBeenCalledWith(WORKSTATION_GUARD_SELECTOR_THEME_ID);
  });

  it("applies a light-biased guard-selector theme from the selected palette", () => {
    const defineTheme = vi.fn();
    const setTheme = vi.fn();
    const tokens = getColorPaletteTokens("factory-light");

    applyWorkstationGuardSelectorTheme(
      {
        editor: { defineTheme, setTheme },
      } as unknown as typeof import("monaco-editor"),
      "factory-light",
    );

    expect(defineTheme).toHaveBeenCalledWith(
      WORKSTATION_GUARD_SELECTOR_THEME_ID,
      expect.objectContaining({
        base: "vs",
        colors: expect.objectContaining({
          "editor.background": tokens.foundation.surface,
          "editor.foreground": tokens.foundation.ink,
        }),
      }),
    );
    expect(setTheme).toHaveBeenCalledWith(WORKSTATION_GUARD_SELECTOR_THEME_ID);
  });
});

describe("buildWorkstationGuardSelectorCompletionItems", () => {
  it("includes curated guard selector suggestions with descriptions", () => {
    expect(buildWorkstationGuardSelectorCompletionItems()).toEqual([
      expect.objectContaining({
        detail: "Match by work name",
        insertText: ".Name",
        label: ".Name",
      }),
      expect.objectContaining({
        detail: "Match by work identifier",
        insertText: ".WorkID",
        label: ".WorkID",
      }),
      expect.objectContaining({
        detail: "Match by tag key",
        insertText: '.Tags["key"]',
        label: '.Tags["key"]',
      }),
    ]);
  });
});

describe("registerWorkstationGuardSelectorCompletionProvider", () => {
  it("scopes suggestions to guard selectors and filters by typed prefix", () => {
    const registerCompletionItemProvider = vi.fn();
    const monaco = {
      languages: {
        CompletionItemKind: { Field: 13 },
        registerCompletionItemProvider,
      },
    } as unknown as typeof import("monaco-editor");

    registerWorkstationGuardSelectorCompletionProvider(monaco);

    const provider = registerCompletionItemProvider.mock.calls[0][1];
    const model = {
      getValueInRange: () => ".Ta",
    };
    const result = provider.provideCompletionItems(model, {
      column: 4,
      lineNumber: 1,
    });

    expect(result.suggestions).toHaveLength(1);
    expect(result.suggestions[0]).toMatchObject({
      insertText: '.Tags["key"]',
      label: '.Tags["key"]',
      detail: "Match by tag key",
    });
    expect(result.suggestions[0].documentation.value).toContain("replace key");
  });

  it("returns all curated suggestions when the selector is empty", () => {
    const registerCompletionItemProvider = vi.fn();
    const monaco = {
      languages: {
        CompletionItemKind: { Field: 13 },
        registerCompletionItemProvider,
      },
    } as unknown as typeof import("monaco-editor");

    registerWorkstationGuardSelectorCompletionProvider(monaco);

    const provider = registerCompletionItemProvider.mock.calls[0][1];
    const model = {
      getValueInRange: () => "",
    };
    const result = provider.provideCompletionItems(model, {
      column: 1,
      lineNumber: 1,
    });

    expect(result.suggestions).toHaveLength(3);
    expect(
      result.suggestions.map((item: { label: string }) => item.label),
    ).toEqual([".Name", ".WorkID", '.Tags["key"]']);
  });

  it("returns no suggestions when the typed prefix matches nothing", () => {
    const registerCompletionItemProvider = vi.fn();
    const monaco = {
      languages: {
        CompletionItemKind: { Field: 13 },
        registerCompletionItemProvider,
      },
    } as unknown as typeof import("monaco-editor");

    registerWorkstationGuardSelectorCompletionProvider(monaco);

    const provider = registerCompletionItemProvider.mock.calls[0][1];
    const model = {
      getValueInRange: () => ".Unknown",
    };
    const result = provider.provideCompletionItems(model, {
      column: 9,
      lineNumber: 1,
    });

    expect(result.suggestions).toEqual([]);
  });
});
// Component lane: requires DOM APIs.
