import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import { resolveWorkerSaveValidationFieldName } from "../../worker-selection/lib/worker-save-validation-field-mapping";
import { resolveWorkstationSaveValidationFieldName } from "../../workstation-selection/lib/workstation-save-validation-field-mapping";
import { mapFactoryValidationTargetsToFieldErrors } from "./map-factory-validation-target-field-errors";

describe("mapFactoryValidationTargetsToFieldErrors", () => {
  it("returns undefined when no targets map to a field", () => {
    const error = new CurrentFactoryDefinitionError("Save failed.", {
      code: "BAD_REQUEST",
      status: 400,
      targets: [
        {
          code: "factory.unknown",
          message: "Save failed.",
          severity: "error",
          subject: {
            id: "unknown",
            location: "DEFINITION",
            type: "FACTORY",
          },
        },
      ],
    });

    expect(
      mapFactoryValidationTargetsToFieldErrors(error, () => null),
    ).toBeUndefined();
  });

  it("deduplicates mapped fields and keeps the error message", () => {
    const error = new CurrentFactoryDefinitionError("Invalid model.", {
      code: "BAD_REQUEST",
      status: 400,
      targets: [
        workerFieldValidationTarget("model", "Invalid model."),
        workerFieldValidationTarget("model", "Duplicate target."),
      ],
    });

    expect(
      mapFactoryValidationTargetsToFieldErrors(
        error,
        resolveWorkerSaveValidationFieldName,
      ),
    ).toEqual({
      model: "Invalid model.",
    });
  });

  it("maps workstation dangling worker references onto workerName", () => {
    const error = new CurrentFactoryDefinitionError(
      "Worker selection must reference a configured worker.",
      {
        code: "BAD_REQUEST",
        status: 400,
        targets: [
          {
            code: "factory.worker.danglingReference",
            message: "Worker selection must reference a configured worker.",
            severity: "error",
            subject: {
              id: "worker",
              location: "DEFINITION",
              type: "WORKSTATION",
            },
          },
        ],
      },
    );

    expect(
      mapFactoryValidationTargetsToFieldErrors(
        error,
        resolveWorkstationSaveValidationFieldName,
      ),
    ).toEqual({
      workerName: "Worker selection must reference a configured worker.",
    });
  });
});
