import { describe, expect, it, vi } from "vitest";

import {
  buildWorkstationPromptMarkers,
  buildWorkstationPromptCompletionItems,
  extractTemplateExpression,
  getCurrentTemplateExpression,
  isInsideTemplate,
  registerWorkstationPromptCompletionProvider,
  resetWorkstationPromptMonacoRegistrationForTests,
  registerWorkstationPromptMonaco,
  WORKSTATION_PROMPT_LANGUAGE_ID,
  WORKSTATION_PROMPT_THEME_ID,
} from "./workstation-prompt-monaco";

describe("registerWorkstationPromptMonaco", () => {
  it("registers the prompt-template language and theme once", () => {
    const register = vi.fn();
    const registerCompletionItemProvider = vi.fn();
    const setMonarchTokensProvider = vi.fn();
    const defineTheme = vi.fn();

    resetWorkstationPromptMonacoRegistrationForTests();

    registerWorkstationPromptMonaco({
      editor: { defineTheme },
      languages: {
        register,
        registerCompletionItemProvider,
        setMonarchTokensProvider,
      },
    } as unknown as typeof import("monaco-editor"));
    registerWorkstationPromptMonaco({
      editor: { defineTheme },
      languages: {
        register,
        registerCompletionItemProvider,
        setMonarchTokensProvider,
      },
    } as unknown as typeof import("monaco-editor"));

    expect(register).toHaveBeenCalledTimes(1);
    expect(register).toHaveBeenCalledWith({
      id: WORKSTATION_PROMPT_LANGUAGE_ID,
    });

    expect(setMonarchTokensProvider).toHaveBeenCalledTimes(1);
    expect(setMonarchTokensProvider).toHaveBeenCalledWith(
      WORKSTATION_PROMPT_LANGUAGE_ID,
      expect.objectContaining({
        defaultToken: "text",
        tokenizer: expect.objectContaining({
          root: expect.arrayContaining([
            [
              expect.objectContaining({ source: "\\{\\{" }),
              expect.objectContaining({
                next: "@template",
                token: "delimiter.template",
              }),
            ],
          ]),
          template: expect.arrayContaining([
            [
              expect.objectContaining({ source: "\\}\\}" }),
              expect.objectContaining({
                next: "@pop",
                token: "delimiter.template",
              }),
            ],
            [expect.objectContaining({ source: "\\.[A-Za-z_]\\w*" }), "variable.root"],
          ]),
        }),
      }),
    );

    expect(defineTheme).toHaveBeenCalledTimes(1);
    expect(defineTheme).toHaveBeenCalledWith(
      WORKSTATION_PROMPT_THEME_ID,
      expect.objectContaining({
        base: "vs-dark",
        inherit: true,
        rules: expect.arrayContaining([
          expect.objectContaining({
            token: "delimiter.template",
          }),
          expect.objectContaining({
            token: "text",
          }),
        ]),
      }),
    );
  });

  it("builds inside-template and full-snippet completion inserts from the prompt-template contract", () => {
    const contract = {
      availableVariables: [
        {
          category: "ROOT",
          description: "Current work identifier.",
          example: "{{ .WorkID }}",
          path: ".WorkID",
        },
        {
          category: "INPUT",
          description: "Payload for the first input.",
          example: "{{ (index .Inputs 0).Payload }}",
          path: ".Inputs[0].Payload",
        },
      ],
      inputCount: 1,
      unavailableAccessPatterns: [],
    } as const;

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: ".WorkID",
        label: ".WorkID",
      }),
      expect.objectContaining({
        insertText: "(index .Inputs 0).Payload",
        label: ".Inputs[0].Payload",
      }),
    ]);

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        insideTemplateExpression: false,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: "{{ .WorkID }}",
      }),
      expect.objectContaining({
        insertText: "{{ (index .Inputs 0).Payload }}",
      }),
    ]);

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: "(index .Inputs 0).",
        currentWordText: "",
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: ".WorkID",
        label: ".WorkID",
      }),
      expect.objectContaining({
        insertText: "Payload",
        label: "Payload",
      }),
    ]);

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: "(index .Inputs 0).Na",
        currentWordText: "Na",
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: ".WorkID",
        label: ".WorkID",
      }),
      expect.objectContaining({
        insertText: "Payload",
        label: "Payload",
      }),
    ]);
  });

  it("detects whether the caret is already inside a template expression", () => {
    expect(isInsideTemplate("Before {{ .WorkID }} after", 12)).toBe(true);
    expect(isInsideTemplate("Before {{ .WorkID }} after", 22)).toBe(false);
    expect(isInsideTemplate("Plain text only", 5)).toBe(false);
  });

  it("extracts the template body from completion snippets", () => {
    expect(extractTemplateExpression("{{ .WorkID }}")).toBe(".WorkID");
    expect(extractTemplateExpression("{{ (index .Inputs 0).Payload }}")).toBe(
      "(index .Inputs 0).Payload",
    );
    expect(extractTemplateExpression(".Prompt")).toBe(".Prompt");
  });

  it("finds the active template expression before the cursor", () => {
    expect(
      getCurrentTemplateExpression("Before {{ (index .Inputs 0).Na", 31),
    ).toBe("(index .Inputs 0).Na");
    expect(getCurrentTemplateExpression("Before {{ .WorkID }} after", 26)).toBe("");
  });

  it("builds Monaco markers from authoritative byte offsets, source-text fallback, and multiline prompts", () => {
    expect(
      buildWorkstationPromptMarkers("😀 {{ .Prompt }}\nSecond {{ .Other }}", [
        {
          endOffset: 18,
          kind: "INVALID_VARIABLE",
          message: "Prompt root is invalid.",
          startOffset: 6,
        },
        {
          kind: "SYNTAX_ERROR",
          message: "Second template is invalid.",
          sourceText: "{{ .Other }}",
        },
      ]),
    ).toEqual([
      expect.objectContaining({
        endColumn: 17,
        endLineNumber: 1,
        message: "Prompt root is invalid.",
        startColumn: 4,
        startLineNumber: 1,
      }),
      expect.objectContaining({
        endColumn: 20,
        endLineNumber: 2,
        message: "Second template is invalid.",
        startColumn: 8,
        startLineNumber: 2,
      }),
    ]);
  });

  it("falls back to a minimal inline marker when a diagnostic range cannot be resolved", () => {
    expect(
      buildWorkstationPromptMarkers("Prompt", [
        {
          kind: "INVALID_VARIABLE",
          message: "Prompt is invalid.",
        },
      ]),
    ).toEqual([
      expect.objectContaining({
        endColumn: 2,
        endLineNumber: 1,
        message: "Prompt is invalid.",
        startColumn: 1,
        startLineNumber: 1,
      }),
    ]);
  });

  it("only returns completion items outside template expressions for manual invocation", () => {
    const registerCompletionItemProvider = vi.fn();
    const completionProvider = {
      dispose: vi.fn(),
    };

    registerCompletionItemProvider.mockReturnValue(completionProvider);

    registerWorkstationPromptCompletionProvider(
      {
        languages: {
          CompletionItemKind: { Variable: 4 },
          CompletionTriggerKind: { Invoke: 0, TriggerCharacter: 1 },
          registerCompletionItemProvider,
        },
      } as unknown as typeof import("monaco-editor"),
      () => ({
        contract: {
          availableVariables: [
            {
              category: "ROOT",
              description: "Current work identifier.",
              example: "{{ .WorkID }}",
              path: ".WorkID",
            },
          ],
          inputCount: 1,
          unavailableAccessPatterns: [],
        },
        status: "ready",
      }),
    );

    const provider = registerCompletionItemProvider.mock.calls[0]?.[1];

    expect(
      provider.provideCompletionItems(
        {
          getOffsetAt: () => 4,
          getValue: () => "Plan",
          getValueInRange: () => "Plan",
          getWordUntilPosition: () => ({
            endColumn: 5,
            startColumn: 1,
          }),
        },
        { column: 5, lineNumber: 1 },
        { triggerKind: 1 },
      ),
    ).toEqual({ suggestions: [] });

    expect(
      provider.provideCompletionItems(
        {
          getOffsetAt: () => 4,
          getValue: () => "Plan",
          getValueInRange: () => "Plan",
          getWordUntilPosition: () => ({
            endColumn: 5,
            startColumn: 1,
          }),
        },
        { column: 5, lineNumber: 1 },
        { triggerKind: 0 },
      ),
    ).toMatchObject({
      suggestions: [
        expect.objectContaining({
          insertText: "{{ .WorkID }}",
          label: ".WorkID",
        }),
      ],
    });
  });
});
