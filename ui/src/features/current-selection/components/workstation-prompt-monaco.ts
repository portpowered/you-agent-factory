import type {
  editor as MonacoEditorAPI,
  languages as MonacoLanguagesAPI,
} from "monaco-editor";

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
