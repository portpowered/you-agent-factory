// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: selection validation scenarios stay grouped around shared projection fixtures.
import { describe, expect, it } from "vitest";

import { CurrentFactoryDefinitionError } from "../../../api/current-factory-definition";
import type { FactoryValidationTarget } from "../../../api/factory-validation";
import { projectFactoryValidationTargets } from "../../factory-graph-editor/lib/projection/factory-validation-graph-projection";
import {
  layoutValidationWarningMessages,
  mergeFactoryValidationTargets,
  saveErrorNoticeMessages,
  validationMessagesForGraphSelection,
  validationMessagesForSelectedWorkstation,
} from "./react-flow-current-activity-card-validation";

describe("validationMessagesForSelectedWorkstation", () => {
  it("returns no messages when the workstation selection is missing", () => {
    expect(
      validationMessagesForSelectedWorkstation({
        projection: projectFactoryValidationTargets([]),
        selectionNodeId: null,
      }),
    ).toEqual([]);
  });

  it("returns no messages for an unknown workstation selection", () => {
    expect(
      validationMessagesForSelectedWorkstation({
        projection: projectFactoryValidationTargets([]),
        selectionNodeId: "missing-workstation",
      }),
    ).toEqual([]);
  });

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

  it("returns workstation handle validation messages for dangling route targets", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.route.danglingPlaceReference",
        message:
          'Workstation "process" references missing place "story:missing-state".',
        severity: "error",
        subject: {
          id: "process",
          location: "OUTPUTS",
          type: "WORKSTATION",
        },
      },
    ];

    const messages = validationMessagesForSelectedWorkstation({
      projection: projectFactoryValidationTargets(targets),
      selectionNodeId: "workstation:process",
    });

    expect(messages).toEqual([
      'Workstation "process" references missing place "story:missing-state".',
    ]);
  });

  it("excludes ON_REJECTION selection messages when continue and reject handles are not rendered", () => {
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

    const messages = validationMessagesForSelectedWorkstation({
      factoryDefinition: {
        name: "alpha",
        workTypes: [],
        workers: [{ name: "worker-a", type: "MODEL_WORKER" }],
        workstations: [
          {
            behavior: "STANDARD",
            inputs: [],
            name: "draft",
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "worker-a",
          },
        ],
      },
      projection: projectFactoryValidationTargets(targets),
      selectionNodeId: "workstation:draft",
    });

    expect(messages).toEqual([
      'Workstation "draft" must define a failure route.',
    ]);
  });

  it("keeps ON_REJECTION selection messages when reject handle is rendered", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.workstation.missingRejectionRoute",
        message: "Workstation process must define a reject route.",
        severity: "error",
        subject: {
          id: "process",
          location: "ON_REJECTION",
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
            behavior: "REPEATER",
            inputs: [],
            name: "process",
            outputs: [],
            type: "MODEL_WORKSTATION",
            worker: "worker-a",
          },
        ],
      },
      projection: projectFactoryValidationTargets(targets),
      selectionNodeId: "workstation:process",
    });

    expect(messages).toEqual([
      "Workstation process must define a reject route.",
    ]);
  });
});

describe("saveErrorNoticeMessages", () => {
  it("returns generic error message text for non-factory errors", () => {
    expect(saveErrorNoticeMessages(new Error("Network save failed"))).toEqual([
      "Network save failed",
    ]);
  });

  it("returns an empty list for non-error values", () => {
    expect(saveErrorNoticeMessages("not-an-error")).toEqual([]);
  });

  it("returns server message text and target messages from save rejections", () => {
    const messages = saveErrorNoticeMessages(
      new CurrentFactoryDefinitionError("The factory definition is invalid.", {
        code: "INVALID_FACTORY_DEFINITION",
        status: 400,
        targets: [
          {
            code: "factory.route.danglingPlaceReference",
            message:
              'Workstation "process" references missing place "story:missing-state".',
            severity: "error",
            subject: {
              id: "process",
              location: "OUTPUTS",
              type: "WORKSTATION",
            },
          },
        ],
      }),
    );

    expect(messages).toEqual([
      "The factory definition is invalid.",
      'Workstation "process" references missing place "story:missing-state".',
    ]);
  });
});

describe("mergeFactoryValidationTargets", () => {
  it("returns an empty projection when no target groups are provided", () => {
    const projection = mergeFactoryValidationTargets();

    expect(projection.workstationMessagesByNodeId.size).toBe(0);
    expect(projection.workTypeMessagesByNodeId.size).toBe(0);
    expect(projection.workStateMessagesByNodeId.size).toBe(0);
  });

  it("projects live validation and save rejection targets together", () => {
    const projection = mergeFactoryValidationTargets(
      [
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
      ],
      [
        {
          code: "factory.route.danglingPlaceReference",
          message:
            'Workstation "process" references missing place "story:missing-state".',
          severity: "error",
          subject: {
            id: "process",
            location: "OUTPUTS",
            type: "WORKSTATION",
          },
        },
      ],
    );

    expect(
      validationMessagesForGraphSelection({
        projection,
        selectionNodeId: "workstation:process",
      }),
    ).toEqual([
      'Workstation "process" references missing place "story:missing-state".',
    ]);
  });
});

describe("validationMessagesForGraphSelection", () => {
  it("merges workstation, work type, and work state messages for the active selection", () => {
    const targets = [
      {
        code: "factory.workstation.missingFailureRoute",
        message: 'Workstation "review" must define a failure route.',
        severity: "error" as const,
        subject: {
          id: "review",
          location: "ON_FAILURE" as const,
          type: "WORKSTATION" as const,
        },
      },
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
      selectionNodeId: "work-type:story",
      selectionPlaceId: "story:queued",
    });

    expect(messages).toEqual([
      'work type "story" must declare a failure state.',
      'work state "story:queued" has no terminal completion path.',
    ]);
  });

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

describe("layoutValidationWarningMessages", () => {
  it("ignores blank messages and non-layout warning targets", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.layout.unknownGroupMemberReference",
        message: "   ",
        severity: "warning",
        subject: {
          id: "review-lane",
          location: "REFERENCE",
          type: "FACTORY",
        },
      },
      {
        code: "factory.layout.invalidGeometry",
        message: "Group bounds are invalid.",
        severity: "error",
        subject: {
          id: "review-lane",
          location: "GEOMETRY",
          type: "FACTORY",
        },
      },
      {
        code: "factory.workstation.missingFailureRoute",
        message: "Workstation review must define a failure route.",
        severity: "warning",
        subject: {
          id: "review",
          location: "ON_FAILURE",
          type: "WORKSTATION",
        },
      },
    ];

    expect(layoutValidationWarningMessages(targets)).toEqual([]);
  });

  it("returns recoverable layout group warnings without topology errors", () => {
    const targets: FactoryValidationTarget[] = [
      {
        code: "factory.layout.unknownGroupMemberReference",
        message:
          'Layout group "review-lane" references unknown graph node "workstation:missing".',
        severity: "warning",
        subject: {
          id: "review-lane",
          location: "REFERENCE",
          type: "FACTORY",
        },
      },
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

    expect(layoutValidationWarningMessages(targets)).toEqual([
      'Layout group "review-lane" references unknown graph node "workstation:missing".',
    ]);
  });
});
