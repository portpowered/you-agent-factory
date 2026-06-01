import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import {
  mapWorkstationSaveErrorToFieldErrors,
  resolveWorkstationSaveValidationFieldName,
} from "./workstation-save-validation-field-mapping";

describe("workstation save validation field mapping", () => {
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
