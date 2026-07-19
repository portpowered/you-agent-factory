import { describe, expect, it } from "vitest";

import { resolveFactorySimpleSubmissionAvailability } from "./factory-simple-submission-eligibility";

describe("resolveFactorySimpleSubmissionAvailability", () => {
  const activeInput = {
    factoryState: "active" as const,
    isCurrent: true,
    workTypes: [
      { handlingBehavior: ["DEFAULT"], isSubmitEligible: true, name: "task" },
    ],
  };

  it("selects exactly one submit-eligible DEFAULT work type", () => {
    expect(resolveFactorySimpleSubmissionAvailability(activeInput)).toEqual({
      kind: "available",
      workTypeName: "task",
    });
  });

  it.each([
    [
      "no default",
      { ...activeInput, workTypes: [{ isSubmitEligible: true, name: "task" }] },
      "no-default",
    ],
    [
      "ineligible default",
      {
        ...activeInput,
        workTypes: [
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: false,
            name: "task",
          },
        ],
      },
      "no-default",
    ],
    [
      "ambiguous defaults",
      {
        ...activeInput,
        workTypes: [
          ...activeInput.workTypes,
          {
            handlingBehavior: ["DEFAULT"],
            isSubmitEligible: true,
            name: "review",
          },
        ],
      },
      "ambiguous-default",
    ],
    [
      "closed factory",
      { ...activeInput, factoryState: "closed" as const },
      "closed",
    ],
    [
      "factory error",
      { ...activeInput, factoryState: "error" as const },
      "error",
    ],
    [
      "invalid factory",
      { ...activeInput, factoryState: "invalid" as const },
      "invalid",
    ],
    [
      "loading factory",
      { ...activeInput, factoryState: "loading" as const },
      "loading",
    ],
    ["history selection", { ...activeInput, isCurrent: false }, "history"],
  ])("returns an unavailable reason for %s", (_caseName, input, reason) => {
    expect(resolveFactorySimpleSubmissionAvailability(input)).toEqual({
      kind: "unavailable",
      reason,
    });
  });
});
