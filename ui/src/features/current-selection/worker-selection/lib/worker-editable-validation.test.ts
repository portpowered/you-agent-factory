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
    name: "reviewer",
    provider: null,
    type: "MODEL_WORKER",
    ...overrides,
  };
}

const validationContext = {
  originalWorkerName: "reviewer",
  workerNames: ["reviewer", "writer"],
};

describe("validateEditableWorkerDraft", () => {
  it("allows model workers with provider set and empty model", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ model: "", modelProvider: "CURSOR" }),
        messages,
      ),
    ).toEqual({});
  });

  it("requires model provider for model workers", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ model: "", modelProvider: null }),
        messages,
        validationContext,
      ),
    ).toEqual({
      modelProvider: messages.editableConfigurationModelProviderRequired,
    });
  });

  it("requires a non-empty worker name", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ name: "   " }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameRequired,
    });
  });

  it("rejects duplicate worker names", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ name: "writer" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameDuplicate("writer"),
    });
  });

  it("allows keeping the current worker name", () => {
    expect(
      validateEditableWorkerDraft(buildDraft(), messages, validationContext),
    ).toEqual({});
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

  it("rejects script worker args that contain invalid characters", () => {
    expect(
      validateEditableWorkerDraft(
        {
          argsText: "check\0lint",
          body: "",
          command: "make check",
          executorProvider: null,
          model: "",
          modelLocality: null,
          modelProvider: null,
          name: "runner",
          provider: null,
          type: "SCRIPT_WORKER",
        },
        messages,
      ),
    ).toEqual({
      args: messages.editableConfigurationArgsInvalid,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: contract validation merge cases stay grouped with decode field mapping.
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
        0,
      ),
    ).toEqual({
      modelProvider: `${messages.editableConfigurationContractInvalidPrefix} factory.workers[0].modelProvider must be one of CLAUDE, CODEX, CURSOR, GEMINI, KIRO, OPENCODE.`,
    });
  });

  it("resolves renamed workers by index for contract validation", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          modelProvider: "Claude",
          name: "senior-reviewer",
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
        0,
      ).modelProvider,
    ).toContain(messages.editableConfigurationContractInvalidPrefix);
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

  it("maps contract failures onto script worker command fields", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          command: 42,
          name: "runner",
          type: "SCRIPT_WORKER",
        },
      ],
      workTypes: [],
    };

    const errors = mergeEditableWorkerContractValidationErrors(
      {},
      pendingFactoryDefinition,
      "runner",
      messages,
    );

    expect(errors.command).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
  });

  it("maps contract failures onto model locality fields", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          modelLocality: "INVALID",
          modelProvider: "CURSOR",
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
      ).modelLocality,
    ).toContain(messages.editableConfigurationContractInvalidPrefix);
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
