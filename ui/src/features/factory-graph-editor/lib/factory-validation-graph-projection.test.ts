import { describe, expect, it } from "vitest";

import type { FactoryValidationTarget } from "../../../api/factory-validation";
import {
  factoryGraphNodeIdForWorkstation,
  projectFactoryValidationTargets,
  validationHandleErrorsForNode,
  workstationHandleIdForValidationLocation,
} from "./factory-validation-graph-projection";

describe("factory-validation-graph-projection", () => {
  it("maps workstation route locations to semantic handle ids", () => {
    expect(workstationHandleIdForValidationLocation("ON_REJECTION")).toBe(
      "workstation-on-rejection-source",
    );
    expect(workstationHandleIdForValidationLocation("ON_FAILURE")).toBe(
      "workstation-on-failure-source",
    );
    expect(workstationHandleIdForValidationLocation("OUTPUTS")).toBe(
      "workstation-output-source",
    );
    expect(workstationHandleIdForValidationLocation("STATES")).toBeNull();
  });

  it("projects workstation validation targets onto graph node handles", () => {
    const targets: FactoryValidationTarget[] = [
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
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "bob" must define a failure route.',
        severity: "error",
        subject: {
          id: "bob",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
      {
        code: "factory.workstation.conflictingWorkStateOutputs",
        message:
          'Workstation "process" routes work type "story" to conflicting output states.',
        severity: "error",
        subject: {
          id: "process",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ];

    const projection = projectFactoryValidationTargets(targets);
    const repeaterNodeId = factoryGraphNodeIdForWorkstation("repeater");

    expect(
      validationHandleErrorsForNode(projection, repeaterNodeId)?.get(
        "workstation-on-rejection-source",
      ),
    ).toEqual({
      code: "factory.workstation.missingRejectionRoute",
      message: "Workstation repeater must define a reject route.",
    });
    expect(
      validationHandleErrorsForNode(
        projection,
        factoryGraphNodeIdForWorkstation("bob"),
      )?.get("workstation-on-failure-source")?.message,
    ).toContain("failure route");
    expect(
      validationHandleErrorsForNode(
        projection,
        factoryGraphNodeIdForWorkstation("process"),
      )?.get("workstation-output-source")?.code,
    ).toBe("factory.workstation.conflictingWorkStateOutputs");
  });
});
