import {
  type ColorPaletteId,
  DEFAULT_COLOR_PALETTE,
} from "../../theme/color-palette";
import { WORKSTATION_GUARD_SELECTOR_MONARCH_LANGUAGE } from "./monaco-guard-selector-language";
import { buildWorkstationGuardSelectorTheme } from "./monaco-theme";

type MonacoModule = typeof import("monaco-editor");

export const WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID =
  "workstation-guard-selector";
export const WORKSTATION_GUARD_SELECTOR_THEME_ID =
  "workstation-guard-selector-theme";

export type WorkstationGuardSelectorCompletionItem = {
  detail: string;
  documentation: string;
  insertText: string;
  label: string;
};

let workstationGuardSelectorMonacoRegistered = false;

export function registerWorkstationGuardSelectorMonaco(monaco: MonacoModule) {
  if (workstationGuardSelectorMonacoRegistered) {
    return;
  }

  monaco.languages.register({ id: WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID });
  monaco.languages.setMonarchTokensProvider(
    WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID,
    WORKSTATION_GUARD_SELECTOR_MONARCH_LANGUAGE,
  );
  applyWorkstationGuardSelectorTheme(monaco);

  workstationGuardSelectorMonacoRegistered = true;
}

export function applyWorkstationGuardSelectorTheme(
  monaco: MonacoModule,
  paletteId: ColorPaletteId = DEFAULT_COLOR_PALETTE,
) {
  monaco.editor.defineTheme(
    WORKSTATION_GUARD_SELECTOR_THEME_ID,
    buildWorkstationGuardSelectorTheme(paletteId),
  );
  monaco.editor.setTheme(WORKSTATION_GUARD_SELECTOR_THEME_ID);
}

export function resetWorkstationGuardSelectorMonacoRegistrationForTests() {
  workstationGuardSelectorMonacoRegistered = false;
}

export function buildWorkstationGuardSelectorCompletionItems(): WorkstationGuardSelectorCompletionItem[] {
  return [
    {
      detail: "Match by work name",
      documentation:
        "Compare grouped inputs using the resolved work item name.",
      insertText: ".Name",
      label: ".Name",
    },
    {
      detail: "Match by work identifier",
      documentation: "Compare grouped inputs using the work item identifier.",
      insertText: ".WorkID",
      label: ".WorkID",
    },
    {
      detail: "Match by tag key",
      documentation:
        "Look up a tag value; replace key with a real tag name (for example _last_output).",
      insertText: '.Tags["key"]',
      label: '.Tags["key"]',
    },
  ];
}

function filterGuardSelectorCompletionItems(
  items: WorkstationGuardSelectorCompletionItem[],
  typedPrefix: string,
): WorkstationGuardSelectorCompletionItem[] {
  const normalizedPrefix = typedPrefix.trim();
  if (normalizedPrefix.length === 0) {
    return items;
  }

  return items.filter(
    (item) =>
      item.label.startsWith(normalizedPrefix) ||
      item.insertText.startsWith(normalizedPrefix),
  );
}

export function registerWorkstationGuardSelectorCompletionProvider(
  monaco: MonacoModule,
) {
  return monaco.languages.registerCompletionItemProvider(
    WORKSTATION_GUARD_SELECTOR_LANGUAGE_ID,
    {
      provideCompletionItems(model, position) {
        const typedPrefix = model.getValueInRange({
          endColumn: position.column,
          endLineNumber: position.lineNumber,
          startColumn: 1,
          startLineNumber: position.lineNumber,
        });
        const replaceRange = {
          endColumn: position.column,
          endLineNumber: position.lineNumber,
          startColumn: 1,
          startLineNumber: position.lineNumber,
        };

        const suggestions = filterGuardSelectorCompletionItems(
          buildWorkstationGuardSelectorCompletionItems(),
          typedPrefix,
        );

        return {
          suggestions: suggestions.map((suggestion, index) => ({
            detail: suggestion.detail,
            documentation: {
              value: suggestion.documentation,
            },
            insertText: suggestion.insertText,
            kind: monaco.languages.CompletionItemKind.Field,
            label: suggestion.label,
            range: replaceRange,
            sortText: `${index.toString().padStart(3, "0")}:${suggestion.label}`,
          })),
        };
      },
      triggerCharacters: ["."],
    },
  );
}
