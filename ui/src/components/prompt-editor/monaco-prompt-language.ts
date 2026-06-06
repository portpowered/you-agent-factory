import type { languages as MonacoLanguagesAPI } from "monaco-editor";

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
