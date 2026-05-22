import { describe, expect, it, vi } from "vitest";

import {
  resetWorkstationPromptMonacoRegistrationForTests,
  registerWorkstationPromptMonaco,
  WORKSTATION_PROMPT_LANGUAGE_ID,
  WORKSTATION_PROMPT_THEME_ID,
} from "./workstation-prompt-monaco";

describe("registerWorkstationPromptMonaco", () => {
  it("registers the prompt-template language and theme once", () => {
    const register = vi.fn();
    const setMonarchTokensProvider = vi.fn();
    const defineTheme = vi.fn();

    resetWorkstationPromptMonacoRegistrationForTests();

    registerWorkstationPromptMonaco({
      editor: { defineTheme },
      languages: { register, setMonarchTokensProvider },
    } as unknown as typeof import("monaco-editor"));
    registerWorkstationPromptMonaco({
      editor: { defineTheme },
      languages: { register, setMonarchTokensProvider },
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
            [expect.objectContaining({ source: "\\.[A-Za-z_]\\w*" }), "variable.root"],
          ]),
        }),
      }),
    );

    expect(defineTheme).toHaveBeenCalledTimes(1);
    expect(defineTheme).toHaveBeenCalledWith(
      WORKSTATION_PROMPT_THEME_ID,
      expect.objectContaining({
        base: "vs",
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
});
