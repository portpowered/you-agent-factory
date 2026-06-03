// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: validation projection scenarios stay grouped around shared workstation fixtures.
import { describe, expect, it } from "vitest";

import type { FactoryValidationTarget } from "../../../api/factory-validation";
import {
  factoryGraphNodeIdForWorkState,
  factoryGraphNodeIdForWorkstation,
  factoryGraphNodeIdForWorkType,
  filterValidationHandleErrorsForWorkstation,
  parseFactoryGraphWorkStateNodeId,
  parseFactoryGraphWorkTypeNodeId,
  projectFactoryValidationTargets,
  validationHandleErrorsForNode,
  validationNodeErrorForNode,
  validationTargetIsRenderedForWorkstation,
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
    expect(parseFactoryGraphWorkTypeNodeId("work-type:story")).toBe("story");
    expect(parseFactoryGraphWorkTypeNodeId("workstation:review")).toBeNull();
    expect(parseFactoryGraphWorkStateNodeId("work-state:story:queued")).toBe(
      "story:queued",
    );
    expect(parseFactoryGraphWorkStateNodeId("work-state:invalid")).toBeNull();
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

  it("filters progress-outcome handle errors using rendered workstation anchors", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation draft must define a reject route.",
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_REJECTION",
          type: "WORKSTATION",
        },
      },
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "draft" must define a failure route.',
        severity: "error",
        subject: {
          id: "draft",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ];
    const projection = projectFactoryValidationTargets(targets);
    const draftNodeId = factoryGraphNodeIdForWorkstation("draft");
    const handleErrors = validationHandleErrorsForNode(projection, draftNodeId);
    if (!handleErrors) {
      throw new Error(
        "expected validation handle errors for draft workstation",
      );
    }

    const standardProcessorWithoutStopWords = {
      type: "MODEL_WORKSTATION" as const,
      behavior: "STANDARD" as const,
    };
    const repeaterWorkstation = {
      type: "MODEL_WORKSTATION" as const,
      behavior: "REPEATER" as const,
    };

    const filteredWithoutStopWords = filterValidationHandleErrorsForWorkstation(
      handleErrors,
      standardProcessorWithoutStopWords,
    );
    expect([...filteredWithoutStopWords.keys()]).toEqual([
      "workstation-on-failure-source",
    ]);
    expect(
      validationTargetIsRenderedForWorkstation(
        targets[0],
        standardProcessorWithoutStopWords,
      ),
    ).toBe(false);
    expect(
      validationTargetIsRenderedForWorkstation(
        targets[1],
        standardProcessorWithoutStopWords,
      ),
    ).toBe(true);

    const filteredForRepeater = filterValidationHandleErrorsForWorkstation(
      handleErrors,
      repeaterWorkstation,
    );
    expect([...filteredForRepeater.keys()]).toEqual([
      "workstation-on-rejection-source",
      "workstation-on-failure-source",
    ]);
    expect(
      validationTargetIsRenderedForWorkstation(targets[0], repeaterWorkstation),
    ).toBe(true);
  });

  it("projects missingOutputRoutes for routeless CRON onto workstation-output-source", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingOutputRoutes",
        message: 'Workstation "cron" must define at least one output route.',
        severity: "error",
        subject: {
          id: "cron",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ];
    const routelessCron = {
      type: "MODEL_WORKSTATION" as const,
      behavior: "CRON" as const,
    };
    const projection = projectFactoryValidationTargets(targets);
    const cronNodeId = factoryGraphNodeIdForWorkstation("cron");

    expect(
      validationHandleErrorsForNode(projection, cronNodeId)?.get(
        "workstation-output-source",
      ),
    ).toEqual({
      code: "factory.workstation.missingOutputRoutes",
      message: 'Workstation "cron" must define at least one output route.',
    });
    expect(
      validationHandleErrorsForNode(projection, cronNodeId)?.has(
        "workstation-on-failure-source",
      ),
    ).toBe(false);

    const handleErrors = validationHandleErrorsForNode(projection, cronNodeId);
    if (!handleErrors) {
      throw new Error("expected validation handle errors for cron workstation");
    }
    expect([
      ...filterValidationHandleErrorsForWorkstation(
        handleErrors,
        routelessCron,
      ).keys(),
    ]).toEqual(["workstation-output-source"]);
    expect(
      validationTargetIsRenderedForWorkstation(targets[0], routelessCron),
    ).toBe(true);
  });

  it("projects missingOutputRoutes for routeless LOGICAL_MOVE onto output handle only", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingOutputRoutes",
        message: 'Workstation "router" must define at least one output route.',
        severity: "error",
        subject: {
          id: "router",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ];
    const routelessLogicalMove = {
      type: "LOGICAL_MOVE" as const,
      behavior: "CRON" as const,
    };
    const projection = projectFactoryValidationTargets(targets);
    const routerNodeId = factoryGraphNodeIdForWorkstation("router");
    const handleErrors = validationHandleErrorsForNode(
      projection,
      routerNodeId,
    );
    if (!handleErrors) {
      throw new Error(
        "expected validation handle errors for logical-move workstation",
      );
    }

    expect(
      filterValidationHandleErrorsForWorkstation(
        handleErrors,
        routelessLogicalMove,
      ).get("workstation-output-source"),
    ).toEqual({
      code: "factory.workstation.missingOutputRoutes",
      message: 'Workstation "router" must define at least one output route.',
    });
    expect(
      filterValidationHandleErrorsForWorkstation(
        handleErrors,
        routelessLogicalMove,
      ).has("workstation-on-failure-source"),
    ).toBe(false);
    expect(
      validationTargetIsRenderedForWorkstation(
        targets[0],
        routelessLogicalMove,
      ),
    ).toBe(true);
  });
});
