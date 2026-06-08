import { describe, expect, it } from "vitest";

import { resolveWorkstationSaveValidationFieldName } from "./workstation-save-validation-field-mapping";

describe("resolveWorkstationSaveValidationFieldName definition fields", () => {
  it("maps dangling worker references and other workstation definition fields", () => {
    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.worker.danglingReference",
        message: "worker is not declared in the factory.",
        severity: "error",
        subject: {
          id: "worker",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("workerName");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.workstation.behavior",
        message: "behavior is invalid.",
        severity: "error",
        subject: {
          id: "behavior",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("behavior");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.workstation.prompt",
        message: "prompt is required.",
        severity: "error",
        subject: {
          id: "body",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("prompt");

    expect(
      resolveWorkstationSaveValidationFieldName({
        code: "factory.workstation.runner",
        message: "runner is required.",
        severity: "error",
        subject: {
          id: "runnerName",
          location: "DEFINITION",
          type: "WORKSTATION",
        },
      }),
    ).toBe("runnerName");
  });
});
