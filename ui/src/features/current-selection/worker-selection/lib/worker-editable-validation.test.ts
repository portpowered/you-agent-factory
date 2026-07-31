// biome-ignore-all lint/style/noExcessiveLinesPerFile lint/complexity/noExcessiveLinesPerFunction: hosted Linear validation regressions share one draft-validation seam.
import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import { EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS } from "../../../current-factory-definition/lib/worker-editable-values";
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
    modelProvider: "CODEX",
    name: "reviewer",
    provider: null,
    skipPermissions: false,
    stopToken: "",
    timeoutAmount: "",
    timeoutUnit: "m",
    type: "MODEL_WORKER",
    ...EMPTY_HOSTED_LINEAR_EDITABLE_DRAFT_FIELDS,
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
        buildDraft({ model: "", modelProvider: "CODEX" }),
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

  it("requires hosted Linear poller fields for LINEAR hosted workers", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({
          model: "",
          modelProvider: null,
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        }),
        messages,
      ),
    ).toEqual({
      authSecretRef: messages.editableConfigurationAuthSecretRefRequired,
      linearMappingState:
        messages.editableConfigurationLinearMappingStateRequired,
      linearMappingWorkType:
        messages.editableConfigurationLinearMappingWorkTypeRequired,
    });
  });

  it("allows hosted Linear workers when required poller fields are set", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({
          authSecretRef: "secrets/linear-api-key",
          linearMappingState: "init",
          linearMappingWorkType: "story",
          model: "",
          modelProvider: null,
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        }),
        messages,
      ),
    ).toEqual({});
  });

  it("allows clearing claim assignee field to remove existing linear.claim config", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({
          authSecretRef: "secrets/linear-api-key",
          linearClaimAssigneeField: "",
          linearMappingState: "init",
          linearMappingWorkType: "story",
          model: "",
          modelProvider: null,
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        }),
        messages,
        validationContext,
      ),
    ).toEqual({});
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
        0,
      ),
    ).toEqual({
      modelProvider: `${messages.editableConfigurationContractInvalidPrefix} factory.workers[0].modelProvider must be a valid provider identity or one of CLAUDE, CODEX, ANTIGRAVITY.`,
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

  it("maps unsupported hosted auth fields onto authSecretRef", () => {
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

    expect(errors.authSecretRef).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
    expect(hasEditableWorkerValidationErrors(errors)).toBe(true);
  });

  it("falls back to a scoped contract banner when the error is not field-specific", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "secrets/linear-api-key" },
          linear: {
            mapping: { workType: "story", state: "init" },
            unsupportedField: "value",
          },
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
          modelProvider: "CODEX",
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
        { name: "reviewer", type: "MODEL_WORKER", modelProvider: "CODEX" },
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

  it("maps contract failures onto skipPermissions", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          name: "reviewer",
          skipPermissions: "yes",
          type: "MODEL_WORKER",
          modelProvider: "CODEX",
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
      ).skipPermissions,
    ).toContain("factory.workers[0].skipPermissions");
  });

  it("maps contract failures onto stopToken", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          name: "reviewer",
          stopToken: 42,
          type: "MODEL_WORKER",
          modelProvider: "CODEX",
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
      ).stopToken,
    ).toContain("factory.workers[0].stopToken");
  });

  it("maps contract failures onto hosted Linear auth secretRef fields", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { clientId: "abc" },
          name: "linear-poller",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    expect(
      mergeEditableWorkerContractValidationErrors(
        {},
        pendingFactoryDefinition,
        "linear-poller",
        messages,
      ).authSecretRef,
    ).toContain("factory.workers[0].auth.clientId");
  });

  it("maps contract failures onto hosted Linear mapping fields", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          auth: { secretRef: "secrets/linear-api-key" },
          linear: { mapping: { workType: 42, state: "init" } },
          name: "linear-poller",
          provider: "LINEAR",
          type: "HOSTED_WORKER",
        },
      ],
      workTypes: [],
    };

    expect(
      mergeEditableWorkerContractValidationErrors(
        {},
        pendingFactoryDefinition,
        "linear-poller",
        messages,
      ).linearMappingWorkType,
    ).toContain("factory.workers[0].linear.mapping.workType");
  });

  it("maps contract failures onto timeout", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          name: "reviewer",
          timeout: 42,
          type: "MODEL_WORKER",
          modelProvider: "CODEX",
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
      ).timeout,
    ).toContain("factory.workers[0].timeout");
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
