import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import * as factoryDefinitionApi from "../../../api/factory-definition";
import { FactoryDefinitionAPIError } from "../../../api/factory-definition";
import {
  hasEditableWorkTypeValidationErrors,
  mergeEditableWorkTypeContractValidationErrors,
  validateEditableWorkTypeDraft,
} from "./work-type-editable-validation";
import type { EditableWorkTypeDraft } from "./work-type-editable-values";

const messages = {
  editableConfigurationContractInvalidPrefix: "Factory contract is invalid.",
  editableConfigurationHandlingBehaviorMultipleDefault:
    "Only one work type can be the factory default.",
  editableConfigurationNameDuplicate: (workTypeName: string) =>
    `Work type name "${workTypeName}" is already in use.`,
  editableConfigurationNameRequired: "Work type name is required.",
};

function buildDraft(
  overrides: Partial<EditableWorkTypeDraft> = {},
): EditableWorkTypeDraft {
  return {
    handlingBehavior: null,
    name: "story",
    ...overrides,
  };
}

const validationContext = {
  originalWorkTypeName: "story",
  workTypeNames: ["story", "bug"],
};

describe("validateEditableWorkTypeDraft", () => {
  it("requires a non-empty work type name", () => {
    expect(
      validateEditableWorkTypeDraft(
        buildDraft({ name: "   " }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameRequired,
    });
  });

  it("rejects duplicate work type names", () => {
    expect(
      validateEditableWorkTypeDraft(
        buildDraft({ name: "bug" }),
        messages,
        validationContext,
      ),
    ).toEqual({
      name: messages.editableConfigurationNameDuplicate("bug"),
    });
  });

  it("allows keeping the current work type name", () => {
    expect(
      validateEditableWorkTypeDraft(buildDraft(), messages, validationContext),
    ).toEqual({});
  });
});

describe("mergeEditableWorkTypeContractValidationErrors", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("surfaces a handlingBehavior error when the pending factory has multiple defaults", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          handlingBehavior: ["DEFAULT"],
          name: "bug",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        {},
        pendingFactoryDefinition,
        messages,
      ),
    ).toEqual({
      handlingBehavior: messages.editableConfigurationHandlingBehaviorMultipleDefault,
    });
    expect(
      hasEditableWorkTypeValidationErrors(
        mergeEditableWorkTypeContractValidationErrors(
          {},
          pendingFactoryDefinition,
          messages,
        ),
      ),
    ).toBe(true);
  });

  it("does not add a duplicate-default error when only one work type is default", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          name: "bug",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        {},
        pendingFactoryDefinition,
        messages,
      ),
    ).toEqual({});
  });

  it("preserves an existing handlingBehavior error over duplicate-default validation", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          handlingBehavior: ["DEFAULT"],
          name: "story",
          states: [{ name: "queued", type: "INITIAL" }],
        },
        {
          handlingBehavior: ["DEFAULT"],
          name: "bug",
          states: [{ name: "open", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        { handlingBehavior: "Existing handlingBehavior error." },
        pendingFactoryDefinition,
        messages,
      ),
    ).toEqual({
      handlingBehavior: "Existing handlingBehavior error.",
    });
  });

  it("surfaces normalizeFactoryDefinition failures on the pending factory", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        {},
        pendingFactoryDefinition,
        messages,
      ),
    ).toMatchObject({
      name: expect.stringContaining(
        messages.editableConfigurationContractInvalidPrefix,
      ),
    });
  });

  it("returns unchanged validation errors when pending factory definition is null", () => {
    expect(
      mergeEditableWorkTypeContractValidationErrors(
        { name: "Work type name is required." },
        null,
        messages,
      ),
    ).toEqual({ name: "Work type name is required." });
  });

  it("maps contract failures onto handlingBehavior when reported by decode", () => {
    vi.spyOn(
      factoryDefinitionApi,
      "normalizeFactoryDefinition",
    ).mockImplementation(() => {
      throw new FactoryDefinitionAPIError(
        "factory.workTypes[0].handlingBehavior must include DEFAULT exactly once.",
      );
    });

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        {},
        {
          name: "Current Factory",
          workTypes: [
            { handlingBehavior: ["INVALID"], name: "story", states: [] },
          ],
        } as CanonicalFactoryDefinition,
        messages,
      ),
    ).toMatchObject({
      handlingBehavior: expect.stringContaining(
        messages.editableConfigurationContractInvalidPrefix,
      ),
    });
  });

  it("falls back to a scoped contract banner when the error is not field-specific", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          name: "story",
          states: [{ name: "queued", type: "NOT_A_REAL_TYPE" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    const errors = mergeEditableWorkTypeContractValidationErrors(
      {},
      pendingFactoryDefinition,
      messages,
    );

    expect(errors.contract).toContain(
      messages.editableConfigurationContractInvalidPrefix,
    );
    expect(hasEditableWorkTypeValidationErrors(errors)).toBe(true);
  });

  it("preserves existing field errors when contract validation fails", () => {
    const pendingFactoryDefinition = {
      name: "Current Factory",
      workTypes: [
        {
          states: [{ name: "queued", type: "INITIAL" }],
        },
      ],
    } as CanonicalFactoryDefinition;

    expect(
      mergeEditableWorkTypeContractValidationErrors(
        { name: "Work type name is required." },
        pendingFactoryDefinition,
        messages,
      ).name,
    ).toBe("Work type name is required.");
  });
});

describe("hasEditableWorkTypeValidationErrors", () => {
  it("returns true when any validation message is present", () => {
    expect(hasEditableWorkTypeValidationErrors({ name: "required" })).toBe(
      true,
    );
    expect(hasEditableWorkTypeValidationErrors({})).toBe(false);
  });
});
