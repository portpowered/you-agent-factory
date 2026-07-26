import type { FactoryValidationResult } from "../../../../api/factory-validation";

export const validationFixtures = {
  repeaterWithoutRejectRoute: {
    targets: [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation repeater must define a reject route.",
        severity: "error",
        subject: {
          id: "repeater",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
    ],
  },
  validFactory: {
    targets: [],
  },
  reviewRemoved: {
    targets: [
      {
        code: "factory.route.danglingPlaceReference",
        message: "Workstation draft references a missing place.",
        severity: "error",
        subject: {
          id: "draft",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ],
  },
  disconnectedFailureRoute: {
    targets: [
      {
        code: "factory.workstation.missingFailureRoute",
        message: "Workstation draft must define a failure route.",
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ],
  },
} satisfies Record<string, FactoryValidationResult>;
