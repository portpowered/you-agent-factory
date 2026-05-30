import { describe, expect, it } from "vitest";

import type { FactoryValidationTarget } from "../../../api/factory-validation";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import {
  validationMessagesForGraphSelection,
  validationMessagesForSelectedWorkstation,
} from "./react-flow-current-activity-card-validation";

describe("validationMessagesForSelectedWorkstation", () => {
  it("returns workstation validation messages for the selected graph node", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "review" must define a failure route.',
        severity: "error",
        subject: {
          id: "review",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ];

    const messages = validationMessagesForSelectedWorkstation({
      factoryDefinition: {
        name: "alpha",
        workTypes: [],
        workers: [{ name: "worker-a", type: "MODEL_WORKER" }],
        workstations: [
          {
            inputs: [],
            name: "review",
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "worker-a",
          },
        ],
      },
      projection: projectFactoryValidationTargets(targets),
      selectionNodeId: "review",
    });

    expect(messages).toEqual([
      'Workstation "review" must define a failure route.',
    ]);
  });
});

describe("validationMessagesForGraphSelection", () => {
  it("returns work type validation messages for the selected graph node", () => {
    const targets = [
      {
        code: "factory.workType.missingFailureState",
        message: 'work type "story" must declare a failure state.',
        severity: "error" as const,
        subject: {
          id: "story",
          location: "STATES" as const,
          type: "WORK_TYPE" as const,
        },
      },
    ];

    const messages = validationMessagesForGraphSelection({
      projection: projectFactoryValidationTargets(targets),
      selectionNodeId: "work-type:story",
    });

    expect(messages).toEqual([
      'work type "story" must declare a failure state.',
    ]);
  });

  it("returns work state validation messages for the selected place", () => {
    const targets = [
      {
        code: "factory.workState.missingTerminalCompletionPath",
        message: 'work state "story:queued" has no terminal completion path.',
        severity: "error" as const,
        subject: {
          id: "story:queued",
          location: "TERMINAL" as const,
          type: "WORK_STATE" as const,
        },
      },
    ];

    const messages = validationMessagesForGraphSelection({
      projection: projectFactoryValidationTargets(targets),
      selectionPlaceId: "story:queued",
    });

    expect(messages).toEqual([
      'work state "story:queued" has no terminal completion path.',
    ]);
  });
});
