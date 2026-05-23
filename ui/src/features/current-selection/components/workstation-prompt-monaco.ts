import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

import type { PromptTemplateContract } from "../../../api/current-factory-prompt-template";
import type { EditableWorkstationPromptDiagnostic } from "../detail-card-types";
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
        const currentWord = model.getWordUntilPosition(position);
        const range = {
          endColumn: currentWord.endColumn,
          endLineNumber: position.lineNumber,
          startColumn: currentWord.startColumn,
          startLineNumber: position.lineNumber,
        };

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

        const currentTemplateExpression = isInsideTemplateExpression
          ? getCurrentTemplateExpression(prompt, cursorOffset)
          : undefined;

        return {
          suggestions: buildWorkstationPromptCompletionItems(
            promptHelpState.contract,
            {
              currentTemplateExpression,
              currentWordText: model.getValueInRange(range),
              insideTemplateExpression: isInsideTemplateExpression,
            },
          ).map((suggestion, index) => ({
            detail: suggestion.detail,
            filterText: suggestion.filterText,
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
    currentTemplateExpression?: string;
    currentWordText?: string;
    insideTemplateExpression: boolean;
  },
) {
  return contract.availableVariables.map((variable) => {
    const fullTemplateExpression = extractTemplateExpression(variable.example).trim();
    const contextualSuggestion = options.insideTemplateExpression
      ? toContextualTemplateSuggestion(
          fullTemplateExpression,
          variable.path,
          options.currentTemplateExpression,
          options.currentWordText,
        )
      : null;

    return {
      detail: `${variable.category} - ${variable.description}`,
      documentation: variable.example,
      filterText: contextualSuggestion?.filterText ?? fullTemplateExpression,
      insertText: options.insideTemplateExpression
        ? (contextualSuggestion?.insertText ?? fullTemplateExpression)
        : variable.example,
      label: contextualSuggestion?.label ?? variable.path,
    };
  });
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

export function getCurrentTemplateExpression(prompt: string, cursorOffset: number) {
  const contentBeforeCursor = prompt.slice(0, cursorOffset);
  const lastOpenIndex = contentBeforeCursor.lastIndexOf("{{");
  const lastCloseIndex = contentBeforeCursor.lastIndexOf("}}");
  if (lastOpenIndex < 0 || lastOpenIndex < lastCloseIndex) {
    return "";
  }

  return contentBeforeCursor.slice(lastOpenIndex + 2).trimStart();
}

function toContextualTemplateSuggestion(
  fullTemplateExpression: string,
  path: string,
  currentTemplateExpression?: string,
  currentWordText?: string,
) {
  const normalizedCurrentExpression = currentTemplateExpression?.trimStart() ?? "";
  if (normalizedCurrentExpression.length === 0) {
    return null;
  }

  const normalizedCurrentWord = currentWordText?.trim() ?? "";
  const expressionBase =
    normalizedCurrentWord.length > 0 &&
    normalizedCurrentExpression.endsWith(normalizedCurrentWord)
      ? normalizedCurrentExpression.slice(0, -normalizedCurrentWord.length)
      : normalizedCurrentExpression;

  if (!fullTemplateExpression.startsWith(expressionBase)) {
    return null;
  }

  const relativeInsertText = fullTemplateExpression.slice(expressionBase.length);

  return {
    filterText: fullTemplateExpression,
    insertText: relativeInsertText,
    label: relativeInsertText.length > 0 ? relativeInsertText : path,
  };
}

export function buildWorkstationPromptMarkers(
  prompt: string,
  diagnostics: EditableWorkstationPromptDiagnostic[],
): MonacoEditorAPI.IMarkerData[] {
  const markers: MonacoEditorAPI.IMarkerData[] = [];

  for (const [index, diagnostic] of diagnostics.entries()) {
    const range =
      resolveWorkstationPromptDiagnosticRange(prompt, diagnostics, diagnostic, index) ??
      fallbackWorkstationPromptDiagnosticRange(prompt);
    if (!range) {
      continue;
    }

    const startPosition = codeUnitIndexToPosition(prompt, range.start);
    const endPosition = codeUnitIndexToPosition(prompt, range.end);

    markers.push({
      code: diagnostic.kind,
      endColumn: endPosition.column,
      endLineNumber: endPosition.lineNumber,
      message: diagnostic.message,
      severity: 8,
      source: "prompt-template-validation",
      startColumn: startPosition.column,
      startLineNumber: startPosition.lineNumber,
    });
  }

  return markers;
}

function fallbackWorkstationPromptDiagnosticRange(prompt: string) {
  if (prompt.length === 0) {
    return null;
  }

  return {
    end: Math.min(prompt.length, 1),
    start: 0,
  };
}

function resolveWorkstationPromptDiagnosticRange(
  prompt: string,
  diagnostics: EditableWorkstationPromptDiagnostic[],
  diagnostic: EditableWorkstationPromptDiagnostic,
  diagnosticIndex: number,
) {
  if (
    typeof diagnostic.startOffset === "number" &&
    typeof diagnostic.endOffset === "number"
  ) {
    const start = utf8ByteOffsetToCodeUnitIndex(prompt, diagnostic.startOffset);
    const end = utf8ByteOffsetToCodeUnitIndex(prompt, diagnostic.endOffset + 1);
    if (start < end) {
      return { end, start };
    }
  }

  if (diagnostic.sourceText) {
    const sourceTextOccurrence = diagnostics
      .slice(0, diagnosticIndex)
      .filter((candidate) => candidate.sourceText === diagnostic.sourceText).length;
    const sourceTextIndex = nthIndexOf(
      prompt,
      diagnostic.sourceText,
      sourceTextOccurrence,
    );
    if (sourceTextIndex >= 0) {
      return {
        end: sourceTextIndex + diagnostic.sourceText.length,
        start: sourceTextIndex,
      };
    }
  }

  return null;
}

function nthIndexOf(text: string, query: string, occurrence: number) {
  let fromIndex = 0;
  let matchIndex = -1;

  for (let index = 0; index <= occurrence; index += 1) {
    matchIndex = text.indexOf(query, fromIndex);
    if (matchIndex < 0) {
      return -1;
    }
    fromIndex = matchIndex + query.length;
  }

  return matchIndex;
}

function utf8ByteOffsetToCodeUnitIndex(text: string, oneBasedByteOffset: number) {
  if (oneBasedByteOffset <= 1) {
    return 0;
  }

  const targetByteCount = oneBasedByteOffset - 1;
  let bytesSeen = 0;

  for (let index = 0; index < text.length; index += 1) {
    const codePoint = text.codePointAt(index);
    if (codePoint == null) {
      return index;
    }

    const codeUnitWidth = codePoint > 0xffff ? 2 : 1;
    const byteWidth =
      codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4;
    if (bytesSeen >= targetByteCount) {
      return index;
    }

    bytesSeen += byteWidth;
    if (bytesSeen >= targetByteCount) {
      return index + codeUnitWidth;
    }

    index += codeUnitWidth - 1;
  }

  return text.length;
}

function codeUnitIndexToPosition(text: string, targetIndex: number) {
  const boundedIndex = Math.max(0, Math.min(targetIndex, text.length));
  let lineNumber = 1;
  let column = 1;

  for (let index = 0; index < boundedIndex; index += 1) {
    if (text[index] === "\n") {
      lineNumber += 1;
      column = 1;
      continue;
    }

    column += 1;
  }

  return { column, lineNumber };
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
  base: "vs-dark",
  colors: {
    "editor.background": "#091117",
    "editor.foreground": "#F7F2E8",
    "editor.lineHighlightBackground": "#101C23",
    "editor.selectionBackground": "#21414A",
    "editor.inactiveSelectionBackground": "#173039",
    "editorCursor.foreground": "#F5C76F",
    "editorWhitespace.foreground": "#FFFFFF24",
    "editorIndentGuide.background1": "#FFFFFF14",
    "editorIndentGuide.activeBackground1": "#FFFFFF2E",
    "editorWidget.background": "#091117",
    "editorWidget.border": "#FFFFFF1F",
    "editorSuggestWidget.background": "#091117",
    "editorSuggestWidget.foreground": "#F7F2E8",
    "editorSuggestWidget.selectedBackground": "#132C37",
    "editorSuggestWidget.highlightForeground": "#F5C76F",
    "editorHoverWidget.background": "#091117",
    "editorHoverWidget.border": "#FFFFFF1F",
  },
  inherit: true,
  rules: [
    { foreground: "F7F2E8", token: "text" },
    { fontStyle: "bold", foreground: "F5C76F", token: "delimiter.template" },
    { foreground: "7DD3FC", token: "keyword.template" },
    { foreground: "B5EDF4", token: "keyword.function.template" },
    { foreground: "F7F2E8", token: "identifier.template" },
    { foreground: "A7F0C4", token: "string.template" },
    { foreground: "F5C76F", token: "number.template" },
    { foreground: "FFB2B2", token: "variable.local" },
    { foreground: "5CCADD", token: "variable.root" },
  ],
};
