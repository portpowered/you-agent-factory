import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

export const WORKSTATION_PROMPT_MONARCH_LANGUAGE: MonacoLanguagesAPI.IMonarchLanguage =
  {
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
        [
          /\b(?:if|else|end|range|with|template|block|define)\b/,
          "keyword.template",
        ],
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

export const WORKSTATION_PROMPT_THEME: MonacoEditorAPI.IStandaloneThemeData = {
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
