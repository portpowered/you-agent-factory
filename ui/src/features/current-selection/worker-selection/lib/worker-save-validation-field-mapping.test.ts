import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../../api/current-factory-definition";
import { workerFieldValidationTarget } from "../../../../testing/factory-validation-target-fixtures";
import {
  mapWorkerSaveErrorToFieldErrors,
  resolveWorkerSaveValidationFieldName,
} from "./worker-save-validation-field-mapping";

describe("resolveWorkerSaveValidationFieldName", () => {
  it("returns null for non-worker validation subjects", () => {
    expect(
      resolveWorkerSaveValidationFieldName({
        subject: { type: "WORKSTATION", id: "timeout" },
        message: "Invalid timeout.",
      }),
    ).toBeNull();
  });

  it("returns null for unrecognized worker field names", () => {
    expect(
      resolveWorkerSaveValidationFieldName(
        workerFieldValidationTarget("unknownField", "Invalid field."),
      ),
    ).toBeNull();
  });

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

  it("maps stopToken save failures from error messages when targets are absent", () => {
    const error = new CurrentFactoryDefinitionError(
      "factory.workers[1].stopToken is invalid.",
      {
        code: "BAD_REQUEST",
        status: 400,
      },
    );

    expect(mapWorkerSaveErrorToFieldErrors(error)).toEqual({
      stopToken: "factory.workers[1].stopToken is invalid.",
    });
  });

  it("maps skipPermissions save failures from error messages when targets are absent", () => {
    const error = new CurrentFactoryDefinitionError(
      "factory.workers[0].skipPermissions must be a boolean.",
      {
        code: "BAD_REQUEST",
        status: 400,
      },
    );

    expect(mapWorkerSaveErrorToFieldErrors(error)).toEqual({
      skipPermissions: "factory.workers[0].skipPermissions must be a boolean.",
    });
  });

  it("returns undefined when the error does not map to worker fields", () => {
    const error = new CurrentFactoryDefinitionError("factory is invalid.", {
      code: "BAD_REQUEST",
      status: 400,
    });

    expect(mapWorkerSaveErrorToFieldErrors(error)).toBeUndefined();
  });
});
