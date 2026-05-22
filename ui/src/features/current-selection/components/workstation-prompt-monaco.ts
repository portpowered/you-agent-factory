import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

import type { PromptTemplateContract } from "../../../api/current-factory-prompt-template";
import type { EditableWorkstationPromptHelpState } from "../detail-card-types";

type MonacoModule = typeof import("monaco-editor");

export const WORKSTATION_PROMPT_LANGUAGE_ID = "workstation-prompt-template";
export const WORKSTATION_PROMPT_THEME_ID = "workstation-prompt-template-theme";

let workstationPromptMonacoRegistered = false;

export function registerWorkstationPromptMonaco(monaco: MonacoModule) {
  if (workstationPromptMonacoRegistered) {
    return;
  }

  monaco.languages.register({ id: WORKSTATION_PROMPT_LANGUAGE_ID });
  monaco.languages.setMonarchTokensProvider(
    WORKSTATION_PROMPT_LANGUAGE_ID,
    WORKSTATION_PROMPT_MONARCH_LANGUAGE,
  );
  monaco.editor.defineTheme(
    WORKSTATION_PROMPT_THEME_ID,
    WORKSTATION_PROMPT_THEME,
  );

  workstationPromptMonacoRegistered = true;
}

export function resetWorkstationPromptMonacoRegistrationForTests() {
  workstationPromptMonacoRegistered = false;
}

export function registerWorkstationPromptCompletionProvider(
  monaco: MonacoModule,
  getPromptHelpState: () => EditableWorkstationPromptHelpState,
) {
  return monaco.languages.registerCompletionItemProvider(
    WORKSTATION_PROMPT_LANGUAGE_ID,
    {
      provideCompletionItems(model, position, _context) {
        const promptHelpState = getPromptHelpState();
        if (promptHelpState.status !== "ready") {
          return { suggestions: [] };
        }

        const prompt = model.getValue();
        const cursorOffset = model.getOffsetAt(position);
        const isManualTrigger = isManualCompletionTrigger(
          monaco,
          _context.triggerKind,
        );
        const isInsideTemplateExpression = isInsideTemplate(prompt, cursorOffset);

        if (!isInsideTemplateExpression && !isManualTrigger) {
          return { suggestions: [] };
        }

        const currentWord = model.getWordUntilPosition(position);
        const range = {
          endColumn: currentWord.endColumn,
          endLineNumber: position.lineNumber,
          startColumn: currentWord.startColumn,
          startLineNumber: position.lineNumber,
        };

        return {
          suggestions: buildWorkstationPromptCompletionItems(
            promptHelpState.contract,
            {
              insideTemplateExpression: isInsideTemplateExpression,
            },
          ).map((suggestion, index) => ({
            detail: suggestion.detail,
            documentation: {
              value: suggestion.documentation,
            },
            insertText: suggestion.insertText,
            kind: monaco.languages.CompletionItemKind.Variable,
            label: suggestion.label,
            range,
            sortText: `${index.toString().padStart(3, "0")}:${suggestion.label}`,
          })),
        };
      },
      triggerCharacters: [".", "$", "("],
    },
  );
}

export function buildWorkstationPromptCompletionItems(
  contract: PromptTemplateContract,
  options: {
    insideTemplateExpression: boolean;
  },
) {
  return contract.availableVariables.map((variable) => ({
    detail: `${variable.category} - ${variable.description}`,
    documentation: variable.example,
    insertText: options.insideTemplateExpression
      ? extractTemplateExpression(variable.example)
      : variable.example,
    label: variable.path,
  }));
}

function isManualCompletionTrigger(
  monaco: MonacoModule,
  triggerKind: MonacoLanguagesAPI.CompletionTriggerKind,
) {
  return triggerKind === monaco.languages.CompletionTriggerKind.Invoke;
}

export function isInsideTemplate(prompt: string, cursorOffset: number) {
  const contentBeforeCursor = prompt.slice(0, cursorOffset);
  const openingDelimiterMatches = contentBeforeCursor.match(/\{\{/g);
  const closingDelimiterMatches = contentBeforeCursor.match(/\}\}/g);
  const openingDelimiterCount = openingDelimiterMatches?.length ?? 0;
  const closingDelimiterCount = closingDelimiterMatches?.length ?? 0;

  return openingDelimiterCount > closingDelimiterCount;
}

export function extractTemplateExpression(example: string) {
  const templateMatch = example.match(/^\s*\{\{\s*([\s\S]*?)\s*\}\}\s*$/);

  return templateMatch?.[1] ?? example;
}

const WORKSTATION_PROMPT_MONARCH_LANGUAGE: MonacoLanguagesAPI.IMonarchLanguage = {
  defaultToken: "text",
  ignoreCase: false,
  tokenizer: {
    root: [
      [/\{\{/, { next: "@template", token: "delimiter.template" }],
      [/[^{}]+/, "text"],
      [/[{}]/, "text"],
    ],
    template: [
      [/\}\}/, { next: "@pop", token: "delimiter.template" }],
      [/\b(?:if|else|end|range|with|template|block|define)\b/, "keyword.template"],
      [
        /\b(?:and|call|eq|ge|gt|html|index|js|le|len|lt|ne|not|or|print|printf|println|slice|urlquery)\b/,
        "keyword.function.template",
      ],
      [/\$[A-Za-z_]\w*/, "variable.local"],
      [/\.[A-Za-z_]\w*/, "variable.root"],
      [/\d+/, "number.template"],
      [/"([^"\\]|\\.)*"/, "string.template"],
      [/'([^'\\]|\\.)*'/, "string.template"],
      [/[|()[\],:=]/, "delimiter.template"],
      [/\s+/, "white"],
      [/[A-Za-z_]\w*/, "identifier.template"],
      [/./, "identifier.template"],
    ],
  },
};

const WORKSTATION_PROMPT_THEME: MonacoEditorAPI.IStandaloneThemeData = {
  base: "vs",
  colors: {},
  inherit: true,
  rules: [
    { foreground: "334155", token: "text" },
    { fontStyle: "bold", foreground: "0369a1", token: "delimiter.template" },
    { foreground: "7c3aed", token: "keyword.template" },
    { foreground: "b45309", token: "keyword.function.template" },
    { foreground: "1d4ed8", token: "identifier.template" },
    { foreground: "0f766e", token: "string.template" },
    { foreground: "475569", token: "number.template" },
    { foreground: "c2410c", token: "variable.local" },
    { foreground: "4338ca", token: "variable.root" },
  ],
};
