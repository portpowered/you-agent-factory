import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import * as factoryDefinitionApi from "../../../../api/factory-definition";
import { FactoryDefinitionAPIError } from "../../../../api/factory-definition";
import type { EditableResourceDraft } from "../../../current-factory-definition/lib/resource-editable-values";
import { getResourceDetailMessages } from "../messages/resource-detail";
import {
  hasEditableResourceValidationErrors,
  mergeEditableResourceContractValidationErrors,
  validateEditableResourceDraft,
} from "./resource-editable-validation";

const messages = getResourceDetailMessages("en");

function buildPendingFactory(
  resources: CanonicalFactoryDefinition["resources"],
): CanonicalFactoryDefinition {
  return {
    name: "Current Factory",
    resources,
    workTypes: [],
  };
}

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

describe("mergeEditableResourceContractValidationErrors", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("maps contract decode failures onto resource capacity when capacity is missing", () => {
    expect(
      mergeEditableResourceContractValidationErrors(
        {},
        buildPendingFactory([{ name: "agent-slot" }]),
        "agent-slot",
        messages,
        0,
      ),
    ).toEqual({
      capacity: expect.stringContaining(
        messages.editableConfigurationContractInvalidPrefix,
      ),
    });
  });

  it("resolves renamed resources by index for contract validation", () => {
    expect(
      mergeEditableResourceContractValidationErrors(
        {},
        buildPendingFactory([{ name: "renamed-slot" }]),
        "agent-slot",
        messages,
        0,
      ).capacity,
    ).toContain(messages.editableConfigurationContractInvalidPrefix);
  });

  it("returns unchanged validation errors when pending factory definition is null", () => {
    expect(
      mergeEditableResourceContractValidationErrors(
        { name: "Name is required." },
        null,
        "agent-slot",
        messages,
      ),
    ).toEqual({ name: "Name is required." });
  });

  it("returns unchanged validation errors when the resource is missing from the pending factory", () => {
    expect(
      mergeEditableResourceContractValidationErrors(
        { name: "Name is required." },
        buildPendingFactory([]),
        "agent-slot",
        messages,
      ),
    ).toEqual({ name: "Name is required." });
  });

  it("falls back to a scoped contract banner when the error is not field-specific", () => {
    vi.spyOn(
      factoryDefinitionApi,
      "normalizeFactoryDefinition",
    ).mockImplementation(() => {
      throw new FactoryDefinitionAPIError("factory.name is required.");
    });

    const errors = mergeEditableResourceContractValidationErrors(
      {},
      buildPendingFactory([{ capacity: 1, name: "agent-slot" }]),
      "agent-slot",
      messages,
    );

    expect(errors.contract).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
    expect(hasEditableResourceValidationErrors(errors)).toBe(true);
  });

  it("preserves existing field errors when contract validation fails", () => {
    expect(
      mergeEditableResourceContractValidationErrors(
        { name: "Name is required." },
        buildPendingFactory([{ name: "agent-slot" }]),
        "agent-slot",
        messages,
        0,
      ).name,
    ).toBe("Name is required.");
  });

  it.each([
    ["backend", "backend"],
    ["loadPolicy", "loadPolicy"],
    ["model", "model"],
    ["provider", "provider"],
    ["type", "type"],
    ["name", "name"],
  ] as const)(
    "maps contract failures onto %s when decode reports factory.resources[0].%s",
    (field) => {
      vi.spyOn(
        factoryDefinitionApi,
        "normalizeFactoryDefinition",
      ).mockImplementation(() => {
        throw new FactoryDefinitionAPIError(
          `factory.resources[0].${field} is invalid.`,
        );
      });

      expect(
        mergeEditableResourceContractValidationErrors(
          {},
          buildPendingFactory([{ capacity: 1, name: "agent-slot" }]),
          "agent-slot",
          messages,
        ),
      ).toMatchObject({
        [field]: expect.stringContaining(
          messages.editableConfigurationContractInvalidPrefix,
        ),
      });
    },
  );
});
