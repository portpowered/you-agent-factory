import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

import type { PromptTemplateContract } from "../../../api/current-factory-prompt-template";
import type {
  EditableWorkstationPromptDiagnostic,
  EditableWorkstationPromptHelpState,
} from "./detail-card-types";
import {
  WORKSTATION_PROMPT_MONARCH_LANGUAGE,
  WORKSTATION_PROMPT_THEME,
} from "./workstation-prompt-monaco-language";

type MonacoModule = typeof import("monaco-editor");
type CompletionInsertMode =
  | "replaceCurrentWord"
  | "insertAtCursor"
  | "replaceExpression";

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
        const isInsideTemplateExpression = isInsideTemplate(
          prompt,
          cursorOffset,
        );

        if (!isInsideTemplateExpression && !isManualTrigger) {
          return { suggestions: [] };
        }

        const currentTemplateExpressionContext = isInsideTemplateExpression
          ? getCurrentTemplateExpressionContext(prompt, cursorOffset)
          : undefined;

        return {
          suggestions: buildWorkstationPromptCompletionItems(
            promptHelpState.contract,
            {
              currentTemplateExpression:
                currentTemplateExpressionContext?.expression,
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
            range: resolveCompletionRange(
              model,
              cursorOffset,
              range,
              suggestion.insertMode,
              currentTemplateExpressionContext?.startOffset,
            ),
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
  return contract.availableVariables.flatMap((variable) => {
    const fullTemplateExpression = extractTemplateExpression(
      variable.example,
    ).trim();
    const contextualSuggestion = options.insideTemplateExpression
      ? toContextualTemplateSuggestion(
          fullTemplateExpression,
          variable.path,
          options.currentTemplateExpression,
        )
      : null;
    if (
      options.insideTemplateExpression &&
      hasTemplateExpressionPrefix(options.currentTemplateExpression) &&
      !contextualSuggestion
    ) {
      return [];
    }

    return [
      {
        detail: `${variable.category} - ${variable.description}`,
        documentation: variable.example,
        filterText: contextualSuggestion?.filterText ?? fullTemplateExpression,
        insertText: options.insideTemplateExpression
          ? (contextualSuggestion?.insertText ?? fullTemplateExpression)
          : variable.example,
        label: contextualSuggestion?.label ?? variable.path,
        insertMode:
          contextualSuggestion?.insertMode ??
          ("replaceCurrentWord" as CompletionInsertMode),
      },
    ];
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

export function getCurrentTemplateExpression(
  prompt: string,
  cursorOffset: number,
) {
  return getCurrentTemplateExpressionContext(prompt, cursorOffset).expression;
}

function getCurrentTemplateExpressionContext(
  prompt: string,
  cursorOffset: number,
) {
  const contentBeforeCursor = prompt.slice(0, cursorOffset);
  const lastOpenIndex = contentBeforeCursor.lastIndexOf("{{");
  const lastCloseIndex = contentBeforeCursor.lastIndexOf("}}");
  if (lastOpenIndex < 0 || lastOpenIndex < lastCloseIndex) {
    return { expression: "", startOffset: cursorOffset };
  }

  const rawExpression = contentBeforeCursor.slice(lastOpenIndex + 2);
  const leadingWhitespaceLength =
    rawExpression.length - rawExpression.trimStart().length;

  return {
    expression: rawExpression.trimStart(),
    startOffset: lastOpenIndex + 2 + leadingWhitespaceLength,
  };
}

function toContextualTemplateSuggestion(
  fullTemplateExpression: string,
  path: string,
  currentTemplateExpression?: string,
) {
  const normalizedCurrentExpression =
    currentTemplateExpression?.trimStart() ?? "";
  if (normalizedCurrentExpression.length === 0) {
    return null;
  }

  if (fullTemplateExpression.startsWith(normalizedCurrentExpression)) {
    const relativeInsertText = fullTemplateExpression.slice(
      normalizedCurrentExpression.length,
    );

    return {
      filterText: fullTemplateExpression,
      insertMode: "insertAtCursor" as const,
      insertText: relativeInsertText,
      label: resolveContextualCompletionLabel(
        normalizedCurrentExpression,
        relativeInsertText,
        path,
      ),
    };
  }

  const mapAccessTargetExpression = getMapAccessTargetExpression(path);
  const normalizedMapPrefix = normalizedCurrentExpression.replace(/\.$/, "");
  if (
    mapAccessTargetExpression &&
    (mapAccessTargetExpression.startsWith(normalizedMapPrefix) ||
      normalizedMapPrefix.startsWith(mapAccessTargetExpression))
  ) {
    return {
      filterText: `${mapAccessTargetExpression} ${fullTemplateExpression}`,
      insertMode: "replaceExpression" as const,
      insertText: fullTemplateExpression,
      label: fullTemplateExpression,
    };
  }

  return null;
}

function resolveContextualCompletionLabel(
  currentExpression: string,
  relativeInsertText: string,
  path: string,
) {
  if (relativeInsertText.length === 0) {
    return path;
  }

  const currentWordMatch = currentExpression.match(/([A-Za-z_]\w*)$/);

  return `${currentWordMatch?.[1] ?? ""}${relativeInsertText}`;
}

function hasTemplateExpressionPrefix(currentTemplateExpression?: string) {
  return (currentTemplateExpression?.trimStart() ?? "").length > 0;
}

function getMapAccessTargetExpression(path: string) {
  if (path === '.Context.Env["KEY"]') {
    return ".Context.Env";
  }

  const inputTagsMatch = path.match(/^\.Inputs\[(\d+)\]\.Tags\["KEY"\]$/);
  if (inputTagsMatch?.[1]) {
    // hardcoded-ui-copy-exception: non-product-diagnostic
    return `(index .Inputs ${inputTagsMatch[1]}).Tags`;
  }

  return null;
}

function resolveCompletionRange(
  model: MonacoEditorAPI.ITextModel,
  cursorOffset: number,
  currentWordRange: MonacoLanguagesAPI.CompletionItem["range"],
  insertMode: CompletionInsertMode,
  expressionStartOffset?: number,
) {
  if (insertMode === "replaceCurrentWord") {
    return currentWordRange;
  }

  const cursorPosition = model.getPositionAt(cursorOffset);
  if (insertMode === "insertAtCursor" || expressionStartOffset === undefined) {
    return {
      endColumn: cursorPosition.column,
      endLineNumber: cursorPosition.lineNumber,
      startColumn: cursorPosition.column,
      startLineNumber: cursorPosition.lineNumber,
    };
  }

  const expressionStartPosition = model.getPositionAt(expressionStartOffset);
  return {
    endColumn: cursorPosition.column,
    endLineNumber: cursorPosition.lineNumber,
    startColumn: expressionStartPosition.column,
    startLineNumber: expressionStartPosition.lineNumber,
  };
}

export function buildWorkstationPromptMarkers(
  prompt: string,
  diagnostics: EditableWorkstationPromptDiagnostic[],
): MonacoEditorAPI.IMarkerData[] {
  const markers: MonacoEditorAPI.IMarkerData[] = [];

  for (const [index, diagnostic] of diagnostics.entries()) {
    const range =
      resolveWorkstationPromptDiagnosticRange(
        prompt,
        diagnostics,
        diagnostic,
        index,
      ) ?? fallbackWorkstationPromptDiagnosticRange(prompt);
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
      .filter(
        (candidate) => candidate.sourceText === diagnostic.sourceText,
      ).length;
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

function utf8ByteOffsetToCodeUnitIndex(
  text: string,
  oneBasedByteOffset: number,
) {
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
      codePoint <= 0x7f
        ? 1
        : codePoint <= 0x7ff
          ? 2
          : codePoint <= 0xffff
            ? 3
            : 4;
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
