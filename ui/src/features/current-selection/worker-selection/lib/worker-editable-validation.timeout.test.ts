import type { EditableWorkerDraft } from "../../../current-factory-definition/lib/worker-editable-values";
import { getWorkerDetailMessages } from "../messages/worker-detail";
import { validateEditableWorkerDraft } from "./worker-editable-validation";

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
    ...overrides,
  };
}

describe("validateEditableWorkerDraft timeout", () => {
  it("rejects non-positive timeout picker values", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ timeoutAmount: "0", timeoutUnit: "m" }),
        messages,
      ),
    ).toEqual({
      timeout: messages.editableConfigurationTimeoutInvalid("0"),
    });
  });

  it("rejects non-numeric timeout picker values", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ timeoutAmount: "abc", timeoutUnit: "s" }),
        messages,
      ),
    ).toEqual({
      timeout: messages.editableConfigurationTimeoutInvalid("abc"),
    });
  });

  it("allows an empty timeout picker without validation errors", () => {
    expect(
      validateEditableWorkerDraft(
        buildDraft({ timeoutAmount: "", timeoutUnit: "m" }),
        messages,
      ),
    ).toEqual({});
  });
});
