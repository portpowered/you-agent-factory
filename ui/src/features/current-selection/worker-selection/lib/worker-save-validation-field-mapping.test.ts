import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import {
  mapWorkerSaveErrorToFieldErrors,
  resolveWorkerSaveValidationFieldName,
} from "./worker-save-validation-field-mapping";

describe("resolveWorkerSaveValidationFieldName", () => {
  it("maps worker runtime validation targets onto editor fields", () => {
    expect(
      resolveWorkerSaveValidationFieldName(
        workerFieldValidationTarget("timeout", "Invalid timeout."),
      ),
    ).toBe("timeout");
    expect(
      resolveWorkerSaveValidationFieldName(
        workerFieldValidationTarget("stopToken", "Invalid stop token."),
      ),
    ).toBe("stopToken");
    expect(
      resolveWorkerSaveValidationFieldName(
        workerFieldValidationTarget("skipPermissions", "Invalid flag."),
      ),
    ).toBe("skipPermissions");
  });
});

describe("mapWorkerSaveErrorToFieldErrors", () => {
  it("maps timeout save failures from validation targets", () => {
    const error = new CurrentFactoryDefinitionError(
      "factory.workers[0].timeout must be a valid Go duration.",
      {
        code: "BAD_REQUEST",
        status: 400,
        targets: [
          workerFieldValidationTarget(
            "timeout",
            "factory.workers[0].timeout must be a valid Go duration.",
          ),
        ],
      },
    );

    expect(mapWorkerSaveErrorToFieldErrors(error)).toEqual({
      timeout: "factory.workers[0].timeout must be a valid Go duration.",
    });
  });

  it("maps runtime field failures from error messages when targets are absent", () => {
    const error = new CurrentFactoryDefinitionError(
      "factory.workers[2].timeout must be a valid Go duration.",
      {
        code: "BAD_REQUEST",
        status: 400,
      },
    );

    expect(mapWorkerSaveErrorToFieldErrors(error)).toEqual({
      timeout: "factory.workers[2].timeout must be a valid Go duration.",
    });
  });
});
