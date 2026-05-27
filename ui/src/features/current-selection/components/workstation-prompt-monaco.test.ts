import { describe, expect, it, vi } from "vitest";

import {
  buildWorkstationPromptCompletionItems,
  buildWorkstationPromptMarkers,
  extractTemplateExpression,
  getCurrentTemplateExpression,
  isInsideTemplate,
  registerWorkstationPromptCompletionProvider,
  registerWorkstationPromptMonaco,
  resetWorkstationPromptMonacoRegistrationForTests,
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
            [
              expect.objectContaining({ source: "\\.[A-Za-z_]\\w*" }),
              "variable.root",
            ],
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
          description: "Human-readable name for the first input.",
          example: "{{ (index .Inputs 0).Name }}",
          path: ".Inputs[0].Name",
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
        insertText: "(index .Inputs 0).Name",
        label: ".Inputs[0].Name",
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
        insertText: "{{ (index .Inputs 0).Name }}",
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
        insertText: "Name",
        label: "Name",
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
        insertText: "me",
        label: "Name",
      }),
    ]);
  });

  it("filters contextual completions to input, nested history, and map prefixes", () => {
    const contract = {
      availableVariables: [
        {
          category: "INPUT",
          description: "Human-readable work name for input 0.",
          example: "{{ (index .Inputs 0).Name }}",
          path: ".Inputs[0].Name",
        },
        {
          category: "INPUT",
          description: "Payload content for input 0.",
          example: "{{ (index .Inputs 0).Payload }}",
          path: ".Inputs[0].Payload",
        },
        {
          category: "HISTORY",
          description: "Retry and failure history for input 0.",
          example: "{{ (index .Inputs 0).History }}",
          path: ".Inputs[0].History",
        },
        {
          category: "HISTORY",
          description: "Current attempt number for input 0.",
          example: "{{ (index .Inputs 0).History.AttemptNumber }}",
          path: ".Inputs[0].History.AttemptNumber",
        },
        {
          category: "HISTORY",
          description: "Last error for input 0.",
          example: "{{ (index .Inputs 0).History.LastError }}",
          path: ".Inputs[0].History.LastError",
        },
        {
          category: "MAP_ACCESS",
          description: "Tag metadata for input 0.",
          example: '{{ index (index .Inputs 0).Tags "branch" }}',
          path: '.Inputs[0].Tags["KEY"]',
        },
        {
          category: "CONTEXT",
          description: "Execution working directory.",
          example: "{{ .Context.WorkDir }}",
          path: ".Context.WorkDir",
        },
        {
          category: "MAP_ACCESS",
          description: "Environment variable access.",
          example: '{{ index .Context.Env "API_KEY" }}',
          path: '.Context.Env["KEY"]',
        },
      ],
      inputCount: 1,
      unavailableAccessPatterns: [],
    } as const;

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: "(index .Inputs 0).",
        currentWordText: "",
        insideTemplateExpression: true,
      }),
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ insertText: "Name", label: "Name" }),
        expect.objectContaining({ insertText: "Payload", label: "Payload" }),
        expect.objectContaining({ insertText: "History", label: "History" }),
      ]),
    );

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: "(index .Inputs 0).History.",
        currentWordText: "",
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: "AttemptNumber",
        label: "AttemptNumber",
      }),
      expect.objectContaining({ insertText: "LastError", label: "LastError" }),
    ]);

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: "(index .Inputs 0).Tags",
        currentWordText: "Tags",
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: 'index (index .Inputs 0).Tags "branch"',
        label: 'index (index .Inputs 0).Tags "branch"',
      }),
    ]);

    expect(
      buildWorkstationPromptCompletionItems(contract, {
        currentTemplateExpression: ".Context.En",
        currentWordText: "En",
        insideTemplateExpression: true,
      }),
    ).toEqual([
      expect.objectContaining({
        insertText: 'index .Context.Env "API_KEY"',
        label: 'index .Context.Env "API_KEY"',
      }),
    ]);
  });

  it("exposes expanded prompt-template contract fields as completion items", () => {
    const contract = {
      availableVariables: [
        {
          category: "INPUT",
          description: "Human-readable work name for input 0.",
          example: "{{ (index .Inputs 0).Name }}",
          path: ".Inputs[0].Name",
        },
        {
          category: "INPUT",
          description: "Data type identifier for input 0.",
          example: "{{ (index .Inputs 0).DataType }}",
          path: ".Inputs[0].DataType",
        },
        {
          category: "INPUT",
          description: "Trace identifier for input 0.",
          example: "{{ (index .Inputs 0).TraceID }}",
          path: ".Inputs[0].TraceID",
        },
        {
          category: "INPUT",
          description: "Parent work identifier for input 0.",
          example: "{{ (index .Inputs 0).ParentID }}",
          path: ".Inputs[0].ParentID",
        },
        {
          category: "INPUT",
          description: "Payload content for input 0.",
          example: "{{ (index .Inputs 0).Payload }}",
          path: ".Inputs[0].Payload",
        },
        {
          category: "INPUT",
          description: "Structured content parts for input 0.",
          example: "{{ (index .Inputs 0).Content }}",
          path: ".Inputs[0].Content",
        },
        {
          category: "INPUT",
          description: "Tag metadata for input 0.",
          example: '{{ index (index .Inputs 0).Tags "branch" }}',
          path: '.Inputs[0].Tags["KEY"]',
        },
        {
          category: "INPUT",
          description: "Relation records attached to input 0.",
          example: "{{ (index .Inputs 0).Relations }}",
          path: ".Inputs[0].Relations",
        },
        {
          category: "INPUT",
          description: "Previous output captured for input 0 retries.",
          example: "{{ (index .Inputs 0).PreviousOutput }}",
          path: ".Inputs[0].PreviousOutput",
        },
        {
          category: "INPUT",
          description: "Reviewer feedback recorded for input 0.",
          example: "{{ (index .Inputs 0).RejectionFeedback }}",
          path: ".Inputs[0].RejectionFeedback",
        },
        {
          category: "HISTORY",
          description: "Retry and failure history for input 0.",
          example: "{{ (index .Inputs 0).History }}",
          path: ".Inputs[0].History",
        },
        {
          category: "HISTORY",
          description: "Current attempt number for input 0.",
          example: "{{ (index .Inputs 0).History.AttemptNumber }}",
          path: ".Inputs[0].History.AttemptNumber",
        },
        {
          category: "HISTORY",
          description: "Total visit count for input 0.",
          example: "{{ (index .Inputs 0).History.TotalVisits }}",
          path: ".Inputs[0].History.TotalVisits",
        },
        {
          category: "HISTORY",
          description: "Failure records captured for input 0.",
          example: "{{ (index .Inputs 0).History.FailureLog }}",
          path: ".Inputs[0].History.FailureLog",
        },
        {
          category: "CONTEXT",
          description: "Execution working directory.",
          example: "{{ .Context.WorkDir }}",
          path: ".Context.WorkDir",
        },
        {
          category: "MAP_ACCESS",
          description: "Environment variable access.",
          example: '{{ index .Context.Env "API_KEY" }}',
          path: '.Context.Env["KEY"]',
        },
      ],
      inputCount: 1,
      unavailableAccessPatterns: [
        {
          example: "{{ (index .Inputs 1).Payload }}",
          path: ".Inputs[N]",
          reason: "Only input 0 is available.",
        },
      ],
    } as const;

    const suggestions = buildWorkstationPromptCompletionItems(contract, {
      insideTemplateExpression: true,
    });
    const labels = suggestions.map((suggestion) => suggestion.label);

    expect(labels).toEqual(
      expect.arrayContaining([
        ".Inputs[0].Name",
        ".Inputs[0].DataType",
        ".Inputs[0].TraceID",
        ".Inputs[0].ParentID",
        ".Inputs[0].Payload",
        ".Inputs[0].Content",
        '.Inputs[0].Tags["KEY"]',
        ".Inputs[0].Relations",
        ".Inputs[0].PreviousOutput",
        ".Inputs[0].RejectionFeedback",
        ".Inputs[0].History",
        ".Inputs[0].History.AttemptNumber",
        ".Inputs[0].History.TotalVisits",
        ".Inputs[0].History.FailureLog",
        ".Context.WorkDir",
        '.Context.Env["KEY"]',
      ]),
    );
    expect(labels).not.toContain(".Inputs[1].Payload");
    expect(suggestions).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          insertText: 'index (index .Inputs 0).Tags "branch"',
          label: '.Inputs[0].Tags["KEY"]',
        }),
        expect.objectContaining({
          insertText: 'index .Context.Env "API_KEY"',
          label: '.Context.Env["KEY"]',
        }),
      ]),
    );
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
    expect(getCurrentTemplateExpression("Before {{ .WorkID }} after", 26)).toBe(
      "",
    );
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
          getPositionAt: () => ({ column: 5, lineNumber: 1 }),
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
          getPositionAt: () => ({ column: 5, lineNumber: 1 }),
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

  it("uses cursor and expression ranges for contextual Monaco completions", () => {
    const registerCompletionItemProvider = vi.fn();
    registerCompletionItemProvider.mockReturnValue({ dispose: vi.fn() });

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
              category: "INPUT",
              description: "Human-readable work name for input 0.",
              example: "{{ (index .Inputs 0).Name }}",
              path: ".Inputs[0].Name",
            },
            {
              category: "MAP_ACCESS",
              description: "Environment variable access.",
              example: '{{ index .Context.Env "API_KEY" }}',
              path: '.Context.Env["KEY"]',
            },
          ],
          inputCount: 1,
          unavailableAccessPatterns: [],
        },
        status: "ready",
      }),
    );

    const provider = registerCompletionItemProvider.mock.calls[0]?.[1];
    const namePrompt = "{{ (index .Inputs 0).Na";

    expect(
      provider.provideCompletionItems(
        {
          getOffsetAt: () => namePrompt.length,
          getPositionAt: (offset: number) => ({
            column: offset + 1,
            lineNumber: 1,
          }),
          getValue: () => namePrompt,
          getValueInRange: () => "Na",
          getWordUntilPosition: () => ({
            endColumn: namePrompt.length + 1,
            startColumn: namePrompt.length - 1,
          }),
        },
        { column: namePrompt.length + 1, lineNumber: 1 },
        { triggerKind: 1 },
      ),
    ).toMatchObject({
      suggestions: [
        expect.objectContaining({
          insertText: "me",
          label: "Name",
          range: {
            endColumn: namePrompt.length + 1,
            endLineNumber: 1,
            startColumn: namePrompt.length + 1,
            startLineNumber: 1,
          },
        }),
      ],
    });

    const envPrompt = "Use {{ .Context.En";

    expect(
      provider.provideCompletionItems(
        {
          getOffsetAt: () => envPrompt.length,
          getPositionAt: (offset: number) => ({
            column: offset + 1,
            lineNumber: 1,
          }),
          getValue: () => envPrompt,
          getValueInRange: () => "En",
          getWordUntilPosition: () => ({
            endColumn: envPrompt.length + 1,
            startColumn: envPrompt.length - 1,
          }),
        },
        { column: envPrompt.length + 1, lineNumber: 1 },
        { triggerKind: 1 },
      ),
    ).toMatchObject({
      suggestions: [
        expect.objectContaining({
          insertText: 'index .Context.Env "API_KEY"',
          range: {
            endColumn: envPrompt.length + 1,
            endLineNumber: 1,
            startColumn: 8,
            startLineNumber: 1,
          },
        }),
      ],
    });
  });
});
