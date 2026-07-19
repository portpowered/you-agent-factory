import { adaptFactorySimpleSubmissionHost } from "./factory-simple-submission-host-adapter";

describe("adaptFactorySimpleSubmissionHost", () => {
  const factory = {
    name: "Example",
    workTypes: [
      { handlingBehavior: ["DEFAULT"], name: "task", states: [] },
      { name: "internal", states: [] },
    ],
  };

  it("joins DEFAULT behavior from the generated Factory contract with projected submit eligibility", () => {
    expect(
      adaptFactorySimpleSubmissionHost({
        factory,
        factoryState: "RUNNING",
        isCurrent: true,
        submitWorkTypes: [{ work_type_name: "task" }],
      }),
    ).toEqual({
      factoryState: "active",
      isCurrent: true,
      workTypes: [
        { handlingBehavior: ["DEFAULT"], isSubmitEligible: true, name: "task" },
        {
          handlingBehavior: undefined,
          isSubmitEligible: false,
          name: "internal",
        },
      ],
    });
  });

  it.each([
    ["IDLE", "active"],
    ["CLOSED", "closed"],
    ["ERROR", "error"],
    ["INVALID", "invalid"],
    ["UNKNOWN", "loading"],
  ] as const)(
    "normalizes %s into the shared availability state %s",
    (factoryState, expected) => {
      expect(
        adaptFactorySimpleSubmissionHost({
          factory: undefined,
          factoryState,
          isCurrent: false,
          submitWorkTypes: [],
        }).factoryState,
      ).toBe(expected);
    },
  );
});
