import { describe, expect, it } from "vitest";

import type { FactoryValidationTarget } from "../../../api/factory-validation";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/factory-validation-graph-projection";
import { validationMessagesForSelectedWorkstation } from "./react-flow-current-activity-card-validation";

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
