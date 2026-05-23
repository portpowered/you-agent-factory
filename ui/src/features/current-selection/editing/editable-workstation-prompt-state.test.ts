// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: existing prompt-state coverage stayed intact during feature-family migration.
import {
  resolvePromptHelpState,
  resolvePromptValidationState,
} from "./editable-workstation-prompt-state";

describe("editable workstation prompt state helpers", () => {
  const messages = {
    editableConfigurationPromptHelpEmpty:
      "Prompt variable help is unavailable.",
    editableConfigurationPromptHelpFallbackError:
      "Prompt variable help failed.",
    editableConfigurationPromptValidationFallbackError:
      "Prompt validation failed.",
  };

  it("maps loading, error, empty, and ready prompt help states", () => {
    expect(
      resolvePromptHelpState(
        { data: undefined, isError: false, isPending: true } as never,
        messages,
      ),
    ).toEqual({ status: "loading" });

    expect(
      resolvePromptHelpState(
        {
          data: undefined,
          error: { message: "" },
          isError: true,
          isPending: false,
        } as never,
        messages,
      ),
    ).toEqual({
      errorMessage: "Prompt variable help failed.",
      status: "error",
    });

    expect(
      resolvePromptHelpState(
        {
          data: {
            availableVariables: [],
            inputCount: 0,
            unavailableAccessPatterns: [],
          },
          isError: false,
          isPending: false,
        } as never,
        messages,
      ),
    ).toEqual({
      message: "Prompt variable help is unavailable.",
      status: "empty",
    });

    expect(
      resolvePromptHelpState(
        {
          data: {
            availableVariables: [
              {
                category: "ROOT",
                description: "Current prompt body.",
                example: "{{ .Prompt }}",
                path: ".Prompt",
              },
            ],
            inputCount: 1,
            unavailableAccessPatterns: [],
          },
          isError: false,
          isPending: false,
        } as never,
        messages,
      ),
    ).toEqual({
      contract: {
        availableVariables: [
          {
            category: "ROOT",
            description: "Current prompt body.",
            example: "{{ .Prompt }}",
            path: ".Prompt",
          },
        ],
        inputCount: 1,
        unavailableAccessPatterns: [],
      },
      status: "ready",
    });
  });

  it("maps idle, loading, error, fallback-error, and ready prompt validation states", () => {
    expect(
      resolvePromptValidationState(
        { data: undefined, isError: false, isPending: false } as never,
        "   ",
        messages,
      ),
    ).toEqual({ status: "idle" });

    expect(
      resolvePromptValidationState(
        { data: undefined, isError: false, isPending: true } as never,
        "Use {{ .Prompt }}",
        messages,
      ),
    ).toEqual({ status: "loading" });

    expect(
      resolvePromptValidationState(
        {
          data: undefined,
          error: { message: "" },
          isError: true,
          isPending: false,
        } as never,
        "Use {{ .Prompt }}",
        messages,
      ),
    ).toEqual({
      errorMessage: "Prompt validation failed.",
      status: "error",
    });

    expect(
      resolvePromptValidationState(
        { data: undefined, isError: false, isPending: false } as never,
        "Use {{ .Prompt }}",
        messages,
      ),
    ).toEqual({
      errorMessage: "Prompt validation failed.",
      status: "error",
    });

    expect(
      resolvePromptValidationState(
        {
          data: {
            diagnostics: [
              {
                endOffset: 24,
                kind: "UNAVAILABLE_VARIABLE",
                message: "Input 1 is unavailable.",
                path: ".Inputs[1]",
                sourceText: "(index .Inputs 1)",
                startOffset: 8,
              },
            ],
            valid: false,
          },
          isError: false,
          isPending: false,
        } as never,
        "Use {{ (index .Inputs 1) }}",
        messages,
      ),
    ).toEqual({
      diagnostics: [
        {
          endOffset: 24,
          kind: "UNAVAILABLE_VARIABLE",
          message: "Input 1 is unavailable.",
          path: ".Inputs[1]",
          sourceText: "(index .Inputs 1)",
          startOffset: 8,
        },
      ],
      result: {
        diagnostics: [
          {
            endOffset: 24,
            kind: "UNAVAILABLE_VARIABLE",
            message: "Input 1 is unavailable.",
            path: ".Inputs[1]",
            sourceText: "(index .Inputs 1)",
            startOffset: 8,
          },
        ],
        valid: false,
      },
      status: "ready",
    });
  });
});
