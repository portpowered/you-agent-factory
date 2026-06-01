import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  hasEditableWorkTypeValidationErrors,
  mergeEditableWorkTypeContractValidationErrors,
} from "./work-type-editable-validation";

const messages = {
  editableConfigurationHandlingBehaviorMultipleDefault:
    "Only one work type can be the factory default.",
};

describe("mergeEditableWorkTypeContractValidationErrors duplicate default", () => {
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
      handlingBehavior:
        messages.editableConfigurationHandlingBehaviorMultipleDefault,
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
});
