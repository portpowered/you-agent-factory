import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import * as factoryDefinitionApi from "../../../api/factory-definition";
import { FactoryDefinitionAPIError } from "../../../api/factory-definition";
import {
  hasEditableWorkTypeValidationErrors,
  mergeEditableWorkTypeContractValidationErrors,
} from "./work-type-editable-validation";

const messages = {
  editableConfigurationContractInvalidPrefix: "Factory contract is invalid.",
};

describe("mergeEditableWorkTypeContractValidationErrors contract decode", () => {
  afterEach(() => {
    vi.restoreAllMocks();
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
