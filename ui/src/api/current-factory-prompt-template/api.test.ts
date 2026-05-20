import { describe, expect, it, vi } from "vitest";

import {
  CurrentFactoryPromptTemplateAPIError,
  getCurrentFactoryWorkstationPromptTemplateContract,
  validateCurrentFactoryWorkstationPromptTemplate,
} from "./api";

describe("current-factory prompt-template API", () => {
  it("loads the current-factory workstation prompt-template contract", async () => {
    const contract = await getCurrentFactoryWorkstationPromptTemplateContract(
      "Review",
      {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              availableVariables: [
                {
                  category: "INPUT",
                  description: "Payload for input 0.",
                  example: "{{ (index .Inputs 0).Payload }}",
                  path: ".Inputs[0].Payload",
                },
              ],
              inputCount: 1,
              unavailableAccessPatterns: [
                {
                  example: "{{ (index .Inputs 1).Payload }}",
                  path: ".Inputs[N]",
                  reason: "Only input 0 is available.",
                },
              ],
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 200,
              statusText: "OK",
            },
          ),
        ),
      },
    );

    expect(contract).toMatchObject({
      inputCount: 1,
      unavailableAccessPatterns: [{ path: ".Inputs[N]" }],
    });
  });

  it("posts prompt validation against the current-factory workstation contract", async () => {
    const fetch = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          diagnostics: [
            {
              endOffset: 18,
              kind: "UNAVAILABLE_VARIABLE",
              message: "Only input 0 is available.",
              path: ".Inputs[1]",
              sourceText: "(index .Inputs 1)",
              startOffset: 18,
            },
          ],
          valid: false,
        }),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
          statusText: "OK",
        },
      ),
    );

    const result = await validateCurrentFactoryWorkstationPromptTemplate(
      "Review",
      "{{ (index .Inputs 1).Payload }}",
      { fetch },
    );

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining(
        "/factory/~current/workstations/Review/prompt-template-validation",
      ),
      expect.objectContaining({
        body: JSON.stringify({ prompt: "{{ (index .Inputs 1).Payload }}" }),
        method: "POST",
      }),
    );
    expect(result).toMatchObject({
      diagnostics: [{ kind: "UNAVAILABLE_VARIABLE", path: ".Inputs[1]" }],
      valid: false,
    });
  });

  it("surfaces typed current-factory prompt-template API errors", async () => {
    let thrown: unknown;

    try {
      await getCurrentFactoryWorkstationPromptTemplateContract("Missing", {
        fetch: vi.fn().mockResolvedValue(
          new Response(
            JSON.stringify({
              code: "NOT_FOUND",
              message: "Current named factory workstation not found.",
            }),
            {
              headers: {
                "Content-Type": "application/json",
              },
              status: 404,
              statusText: "Not Found",
            },
          ),
        ),
      });
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(CurrentFactoryPromptTemplateAPIError);
    expect(thrown).toMatchObject({
      code: "NOT_FOUND",
      message: "Current named factory workstation not found.",
      status: 404,
      statusText: "Not Found",
    });
  });
});
