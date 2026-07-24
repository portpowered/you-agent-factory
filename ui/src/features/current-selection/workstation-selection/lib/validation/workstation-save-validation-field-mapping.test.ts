import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../../api/current-factory-definition";
import type { FactoryValidationTarget } from "../../../../../api/factory-validation";
import {
  mapWorkstationSaveErrorToFieldErrors,
  resolveWorkstationSaveValidationFieldName,
} from "./workstation-save-validation-field-mapping";

function workstationCronFieldValidationTarget(
  subjectID: string,
  message: string,
): FactoryValidationTarget {
  return {
    code: `factory.workstation.cron.${subjectID}`,
    message,
    severity: "error",
    subject: {
      id: subjectID,
      location: "DEFINITION",
      type: "WORKSTATION",
    },
  };
}

function workstationCronPathValidationTarget(
  pathSubjectID: string,
  message: string,
): FactoryValidationTarget {
  return {
    code: "cron-schedule",
    message,
    severity: "error",
    subject: {
      id: pathSubjectID,
      location: "DEFINITION",
      type: "WORKSTATION",
    },
  };
}

describe("resolveWorkstationSaveValidationFieldName", () => {
  it.each([
    ["cron.schedule", "cronSchedule"],
    ["workstations[0](daily-refresh).cron.schedule", "cronSchedule"],
    ["cron.jitter", "cronJitter"],
    ["workstations[2](poll).cron.jitter", "cronJitter"],
    ["cron.expiry_window", "cronExpiryWindow"],
    ["cron.expiryWindow", "cronExpiryWindow"],
    ["workstations[0](daily-refresh).cron.expiry_window", "cronExpiryWindow"],
    ["cron.trigger_at_start", "cronTriggerAtStart"],
    ["cron.triggerAtStart", "cronTriggerAtStart"],
  ] as const)("maps cron subject %s to %s", (subjectID, fieldName) => {
    expect(
      resolveWorkstationSaveValidationFieldName(
        workstationCronFieldValidationTarget(subjectID, "Invalid value."),
      ),
    ).toBe(fieldName);
  });

  it("returns null for unrelated workstation subjects", () => {
    expect(
      resolveWorkstationSaveValidationFieldName(
        workstationCronFieldValidationTarget("outputs", "Invalid outputs."),
      ),
    ).toBeNull();
  });

  it("maps workstation name validation targets onto the name field", () => {
    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.duplicateIdentifier",
        message: 'duplicate workstation name "Plan".',
        severity: "error",
        subject: {
          id: "Plan",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("name");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.workstation.name",
        message: "workstation name must be non-empty.",
        severity: "error",
        subject: {
          id: "name",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("name");
  });

  it("maps model invoke operation and binding validation targets", () => {
    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "workstation-model-invoke-operation",
        message:
          "MODEL_INVOKE workstation requires an uppercase operation name.",
        severity: "error",
        subject: {
          id: "operation",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("operation");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "workstation-model-invoke-binding-empty",
        message:
          "operation binding must declare a selector, config content, or default content",
        severity: "error",
        subject: {
          id: "operationBindings",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("operationBindings");
  });

  it("maps guard validation target codes onto guard field errors", () => {
    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "guard-visit-count-max-visits",
        message: "Max visits must be positive.",
        severity: "error",
        subject: {
          id: "guards[0]",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("guards[0].maxVisits");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "per-input-guard-match-input",
        message: "Match input is required.",
        severity: "error",
        subject: {
          id: "inputs[1].guard",
          location: "INPUTS",
          type: "WORKSTATION",
        },
      }),
    ).toBe("inputs[1].guard.matchInput");
  });
});

describe("mapWorkstationSaveErrorToFieldErrors", () => {
  it("maps cron validation targets onto cron form field keys", () => {
    const error = new CurrentFactoryDefinitionError("Save failed.", {
      code: "BAD_REQUEST",
      status: 400,
      targets: [
        workstationCronPathValidationTarget(
          "workstations[0](daily-refresh).cron.schedule",
          "cron workstation requires non-empty 'schedule'",
        ),
        workstationCronFieldValidationTarget("cron.jitter", "jitter invalid"),
        workstationCronFieldValidationTarget(
          "cron.expiryWindow",
          "expiry_window invalid",
        ),
        workstationCronFieldValidationTarget(
          "cron.triggerAtStart",
          "triggerAtStart unsupported.",
        ),
      ],
    });

    expect(mapWorkstationSaveErrorToFieldErrors(error)).toEqual({
      cronExpiryWindow: "Save failed.",
      cronJitter: "Save failed.",
      cronSchedule: "Save failed.",
      cronTriggerAtStart: "Save failed.",
    });
  });

  it("maps factory.workstations name paths onto the name field", () => {
    expect(
      mapWorkstationSaveErrorToFieldErrors(
        new CurrentFactoryDefinitionError(
          "factory.workstations[1].name must be non-empty.",
          {
            code: "BAD_REQUEST",
            status: 400,
          },
        ),
      ),
    ).toEqual({
      name: "factory.workstations[1].name must be non-empty.",
    });
  });

  it("maps model invoke operation and binding save messages onto field errors", () => {
    expect(
      mapWorkstationSaveErrorToFieldErrors(
        new CurrentFactoryDefinitionError(
          "factory.workstations[0](speak).operation is required.",
          {
            code: "BAD_REQUEST",
            status: 400,
          },
        ),
      ),
    ).toEqual({
      operation: "factory.workstations[0](speak).operation is required.",
    });

    expect(
      mapWorkstationSaveErrorToFieldErrors(
        new CurrentFactoryDefinitionError(
          "factory.workstations[0](speak).operationBindings[1](text) must declare content.",
          {
            code: "BAD_REQUEST",
            status: 400,
          },
        ),
      ),
    ).toEqual({
      operationBindings:
        "factory.workstations[0](speak).operationBindings[1](text) must declare content.",
    });
  });

  it("maps structured guard save failures and factory guard paths onto field errors", () => {
    expect(
      mapWorkstationSaveErrorToFieldErrors(
        new CurrentFactoryDefinitionError(
          "factory.workstations[0].guards[0].maxVisits must be an integer.",
          {
            code: "BAD_REQUEST",
            status: 400,
            targets: [
              {
                code: "guard-visit-count-max-visits",
                message:
                  "factory.workstations[0].guards[0].maxVisits must be an integer.",
                severity: "error",
                subject: {
                  id: "guards[0]",
                  location: "DEFINITION",
                  type: "WORKSTATION",
                },
              },
            ],
          },
        ),
      ),
    ).toEqual({
      "guards[0].maxVisits":
        "factory.workstations[0].guards[0].maxVisits must be an integer.",
    });

    expect(
      mapWorkstationSaveErrorToFieldErrors(
        new CurrentFactoryDefinitionError(
          "factory.workstations[0].inputs[1].guards[0].matchInput is required.",
          {
            code: "BAD_REQUEST",
            status: 400,
          },
        ),
      ),
    ).toEqual({
      "inputs[1].guard.matchInput":
        "factory.workstations[0].inputs[1].guards[0].matchInput is required.",
    });
  });
});
