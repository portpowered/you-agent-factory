import { describe, expect, it } from "vitest";

import type { FactoryValidationTarget } from "../../../api/factory-validation";
import {
  factoryGraphNodeIdForWorkState,
  factoryGraphNodeIdForWorkstation,
  factoryGraphNodeIdForWorkType,
  projectFactoryValidationTargets,
  validationHandleErrorsForNode,
  validationNodeErrorForNode,
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
    expect(workstationHandleIdForValidationLocation("TERMINAL")).toBeNull();
  });

  it("maps work type and work state subject ids to graph node ids", () => {
    expect(factoryGraphNodeIdForWorkType("story")).toBe("work-type:story");
    expect(factoryGraphNodeIdForWorkState("story:queued")).toBe(
      "work-state:story:queued",
    );
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

  it("projects work type and work state validation targets onto graph nodes", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workType.missingCompletionState",
        message: 'work type "story" must declare a completion state.',
        severity: "error",
        subject: {
          id: "story",
          location: "STATES",
          type: "WORK_TYPE",
        },
      },
      {
        code: "factory.workState.missingTerminalCompletionPath",
        message: 'work state "story:queued" has no terminal completion path.',
        severity: "error",
        subject: {
          id: "story:queued",
          location: "TERMINAL",
          type: "WORK_STATE",
        },
      },
    ];

    const projection = projectFactoryValidationTargets(targets);

    expect(validationNodeErrorForNode(projection, "work-type:story")).toEqual({
      code: "factory.workType.missingCompletionState",
      message: 'work type "story" must declare a completion state.',
    });
    expect(
      validationNodeErrorForNode(projection, "work-state:story:queued"),
    ).toEqual({
      code: "factory.workState.missingTerminalCompletionPath",
      message: 'work state "story:queued" has no terminal completion path.',
    });
  });
});
