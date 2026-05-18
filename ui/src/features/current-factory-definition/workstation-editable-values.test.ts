import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import {
  applyEditableWorkstationDraft,
  resolveEditableWorkstationValues,
} from "./workstation-editable-values";

const selectedNode: DashboardWorkstationNode = {
  model: "gpt-5.4",
  node_id: "review",
  transition_id: "review",
  workstation_kind: "MODEL_WORKSTATION",
  workstation_name: "Review",
};

describe("resolveEditableWorkstationValues", () => {
  it("joins the selected workstation with the canonical worker model", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      isModelEditable: true,
      model: "gpt-5.5",
      modelEditBlockedReason: null,
      prompt: "Review the latest story changes before approval.",
      promptFile: "prompts/review.md",
      workerName: "reviewer",
      workstationName: "Review",
    });
  });

  it("falls back from transition id lookup to workstation name lookup", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "reviewer",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)?.workstationName,
    ).toBe("Review");
  });

  it("returns null when the selected workstation has no canonical worker", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "missing-worker",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toBeNull();
  });

  it("applies editable draft changes without rewriting unsupported workstation fields", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.4",
          name: "reviewer",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review the latest story changes before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          limits: { maxRetries: 3 },
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          promptFile: "prompts/review.md",
          stopWords: ["STOP"],
          worker: "reviewer",
          workingDirectory: "/repo/review",
        },
      ],
      workTypes: [],
    };

    const updatedFactory = applyEditableWorkstationDraft(
      factory,
      selectedNode,
      {
        model: "gpt-5.5",
        prompt: "Review the updated prompt before approval.",
        promptFile: "prompts/review-updated.md",
      },
    );

    expect(updatedFactory).toMatchObject({
      workers: [
        {
          body: "existing worker body",
          model: "gpt-5.5",
          name: "reviewer",
        },
      ],
      workstations: [
        {
          body: "Review the updated prompt before approval.",
          guards: [{ maxVisits: 1, type: "VISIT_COUNT" }],
          limits: { maxRetries: 3 },
          promptFile: "prompts/review-updated.md",
          stopWords: ["STOP"],
          workingDirectory: "/repo/review",
        },
      ],
    });
  });

  it("marks model edits as non-workstation-scoped when the worker is shared", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(resolveEditableWorkstationValues(factory, selectedNode)).toEqual({
      isModelEditable: false,
      model: "gpt-5.4",
      modelEditBlockedReason:
        'Model edits are disabled here because worker "processor" is shared with "Review" and "Plan".',
      prompt: "Review work",
      promptFile: null,
      workerName: "processor",
      workstationName: "Review",
    });
  });

  it("lists every sibling workstation when a worker is shared by more than two workstations", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Code work",
          id: "code",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Code",
          outputs: [{ state: "implemented", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      resolveEditableWorkstationValues(factory, selectedNode)
        ?.modelEditBlockedReason,
    ).toBe(
      'Model edits are disabled here because worker "processor" is shared with "Review", "Plan", and "Code".',
    );
  });

  it("rejects shared-worker model rewrites while still allowing workstation-only edits", () => {
    const factory: CanonicalFactoryDefinition = {
      name: "Current Factory",
      workers: [
        {
          model: "gpt-5.4",
          name: "processor",
          type: "MODEL_WORKER",
        },
      ],
      workstations: [
        {
          body: "Review work",
          id: "review",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Review",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
        {
          body: "Plan work",
          id: "plan",
          inputs: [{ state: "queued", workType: "story" }],
          name: "Plan",
          outputs: [{ state: "approved", workType: "story" }],
          worker: "processor",
        },
      ],
      workTypes: [],
    };

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        model: "gpt-5.5",
        prompt: "Updated review work",
        promptFile: "",
      }),
    ).toBeNull();

    expect(
      applyEditableWorkstationDraft(factory, selectedNode, {
        model: "gpt-5.4",
        prompt: "Updated review work",
        promptFile: "",
      }),
    ).toMatchObject({
      workers: [{ model: "gpt-5.4", name: "processor" }],
      workstations: [
        { body: "Updated review work", name: "Review" },
        { body: "Plan work", name: "Plan" },
      ],
    });
  });
});
