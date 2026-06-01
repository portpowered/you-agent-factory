import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import type { FactoryValidationTarget } from "../../../../api/factory-validation";
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
  ] as const)("maps %s to %s", (subjectID, fieldName) => {
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
});

describe("mapWorkstationSaveErrorToFieldErrors", () => {
  it("maps cron validation targets onto cron form field keys", () => {
    const scheduleMessage = "cron workstation requires non-empty 'schedule'";
    const jitterMessage = 'jitter must be a non-negative duration, got "bad"';
    const error = new CurrentFactoryDefinitionError("Save failed.", {
      code: "BAD_REQUEST",
      status: 400,
      targets: [
        workstationCronPathValidationTarget(
          "workstations[0](daily-refresh).cron.schedule",
          scheduleMessage,
        ),
        workstationCronFieldValidationTarget("cron.jitter", jitterMessage),
        workstationCronFieldValidationTarget(
          "cron.expiryWindow",
          'expiry_window must be a positive duration, got "0s"',
        ),
        workstationCronFieldValidationTarget(
          "cron.triggerAtStart",
          "triggerAtStart is not supported in this fixture.",
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
});
