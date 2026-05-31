import type { EditableResourceDraft } from "../../../current-factory-definition/lib/resource-editable-values";
import { getResourceDetailMessages } from "../messages/resource-detail";
import {
  hasEditableResourceValidationErrors,
  validateEditableResourceDraft,
} from "./resource-editable-validation";

function buildDraft(
  overrides: Partial<EditableResourceDraft> = {},
): EditableResourceDraft {
  return {
    backend: "",
    capacityText: "2",
    loadPolicy: "",
    model: "",
    name: "agent-slot",
    provider: "",
    type: "INVOCATION_SLOT",
    ...overrides,
  };
}

describe("validateEditableResourceDraft", () => {
  const messages = getResourceDetailMessages("en");
  const validationContext = {
    originalResourceName: "agent-slot",
    resourceNames: ["agent-slot", "voice-model"],
  };

  it("requires a non-empty resource name", () => {
    expect(
      validateEditableResourceDraft(
        buildDraft({ name: "   " }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameRequired,
    });
  });

  it("blocks rename collisions with another resource name", () => {
    expect(
      validateEditableResourceDraft(
        buildDraft({ name: "voice-model" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameDuplicate("voice-model"),
    });
  });

  it("allows keeping the original resource name", () => {
    expect(
      validateEditableResourceDraft(
        buildDraft({ name: "agent-slot" }),
        messages,
        validationContext,
      ),
    ).toEqual({});
  });

  it("requires capacity to be an integer greater than zero", () => {
    expect(
      validateEditableResourceDraft(
        buildDraft({ capacityText: "0" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      capacity: messages.editableConfigurationCapacityInvalid,
    });

    expect(
      validateEditableResourceDraft(
        buildDraft({ capacityText: "abc" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      capacity: messages.editableConfigurationCapacityInvalid,
    });
  });
});

describe("hasEditableResourceValidationErrors", () => {
  it("returns false for an empty error map", () => {
    expect(hasEditableResourceValidationErrors({})).toBe(false);
  });

  it("returns true when any field error is present", () => {
    expect(
      hasEditableResourceValidationErrors({
        capacity: "Capacity must be a whole number greater than zero.",
      }),
    ).toBe(true);
  });
});
