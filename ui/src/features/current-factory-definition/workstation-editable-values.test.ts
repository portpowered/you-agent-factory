import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import { resolveEditableWorkstationValues } from "./workstation-editable-values";

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
      model: "gpt-5.5",
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

    expect(resolveEditableWorkstationValues(factory, selectedNode)?.workstationName).toBe("Review");
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
});
