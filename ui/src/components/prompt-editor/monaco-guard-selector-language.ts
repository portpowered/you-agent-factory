import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

export const WORKSTATION_GUARD_SELECTOR_MONARCH_LANGUAGE: MonacoLanguagesAPI.IMonarchLanguage =
  {
    defaultToken: "text",
    ignoreCase: false,
    tokenizer: {
      root: [
        [/\.Tags\["([^"\\]|\\.)*"\]/, "selector.tag"],
        [/\.[A-Za-z_]\w*/, "selector.field"],
        [/./, "text"],
      ],
    },
  };

export const WORKSTATION_GUARD_SELECTOR_THEME: MonacoEditorAPI.IStandaloneThemeData =
  {
    base: "vs-dark",
    colors: {
      "editor.background": "#091117",
      "editor.foreground": "#F7F2E8",
      "editor.lineHighlightBackground": "#101C23",
      "editor.selectionBackground": "#21414A",
      "editor.inactiveSelectionBackground": "#173039",
      "editorCursor.foreground": "#F5C76F",
      "editorWidget.background": "#091117",
      "editorWidget.border": "#FFFFFF1F",
      "editorSuggestWidget.background": "#091117",
      "editorSuggestWidget.foreground": "#F7F2E8",
      "editorSuggestWidget.selectedBackground": "#132C37",
      "editorSuggestWidget.highlightForeground": "#F5C76F",
    },
    inherit: true,
    rules: [
      { foreground: "F7F2E8", token: "text" },
      { foreground: "5CCADD", token: "selector.field" },
      { foreground: "A7F0C4", token: "selector.tag" },
    ],
  };
