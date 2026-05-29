import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import {
  hasEditableWorkerValidationErrors,
  mergeEditableWorkerContractValidationErrors,
  validateEditableWorkerDraft,
} from "./worker-editable-validation";

const messages = getWorkerDetailMessages("en");

function buildDraft(
  overrides: Partial<EditableWorkerDraft> = {},
): EditableWorkerDraft {
  return {
    argsText: "",
    body: "",
    command: "",
    executorProvider: null,
    model: "gpt-5.5",
    modelLocality: null,
    modelProvider: "CURSOR",
    provider: null,
    type: "MODEL_WORKER",
    ...overrides,
  };
}

describe("validateEditableWorkerDraft", () => {
  it("requires model provider and model for model workers", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ model: "", modelProvider: null }),
        messages,
      ),
    ).toEqual({
      model: messages.editableConfigurationModelRequired,
      modelProvider: messages.editableConfigurationModelProviderRequired,
    });
  });

  it("requires command or body for script workers", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({
          command: "",
          body: "",
          model: "",
          modelProvider: null,
          type: "SCRIPT_WORKER",
        }),
        messages,
      ),
    ).toEqual({
      body: messages.editableConfigurationScriptCommandOrBodyRequired,
      command: messages.editableConfigurationScriptCommandOrBodyRequired,
    });
  });

  it("requires hosted provider for hosted workers", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({
          model: "",
          provider: null,
          type: "HOSTED_WORKER",
        }),
        messages,
      ),
    ).toEqual({
      provider: messages.editableConfigurationProviderRequired,
    });
  });
});

describe("mergeEditableWorkerContractValidationErrors", () => {
  it("maps contract decode failures onto worker fields when possible", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          modelProvider: "Claude",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workTypes: [],
    };

    expect(
      mergeEditableWorkerContractValidationErrors(
        {},
        pendingFactoryDefinition,
        "reviewer",
        messages,
      ),
    ).toEqual({
      modelProvider: `${messages.editableConfigurationContractInvalidPrefix} factory.workers[0].modelProvider must be one of CLAUDE, CODEX, CURSOR, GEMINI, KIRO, OPENCODE.`,
    });
  });

  it("falls back to a scoped contract banner when the error is not field-specific", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { clientId: "secret" },
          name: "reviewer",
          type: "HOSTED_WORKER",
          provider: "LINEAR",
        },
      ],
      workTypes: [],
    };

    const errors = mergeEditableWorkerContractValidationErrors(
      {},
      pendingFactoryDefinition,
      "reviewer",
      messages,
    );

    expect(errors.contract).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
    expect(hasEditableWorkerValidationErrors(errors)).toBe(true);
  });

  it("preserves existing field errors when contract validation fails", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        { name: "reviewer", type: "MODEL_WORKER", modelProvider: "CURSOR" },
      ],
      workTypes: [],
    };

    expect(
      mergeEditableWorkerContractValidationErrors(
        { model: "Model is required." },
        pendingFactoryDefinition,
        "reviewer",
        messages,
      ).model,
    ).toBe("Model is required.");
  });
});

describe("hasEditableWorkerValidationErrors", () => {
  it("returns false for an empty error map", () => {
    expect(hasEditableWorkerValidationErrors({})).toBe(false);
  });

  it("returns true when any field error is present", () => {
    expect(
      hasEditableWorkerValidationErrors({
        model: "Model is required.",
      }),
    ).toBe(true);
  });
});
