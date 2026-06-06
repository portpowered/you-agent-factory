import type { languages as MonacoLanguagesAPI } from "monaco-editor";

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
